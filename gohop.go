package gohop

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"net/http"

	"bosun.org/opentsdb"
)

type Client struct {
	apiKey string
	apiURL string
}

// NewClient creates an instance of a ExtraHop REST API v1 client.
func NewClient(apiURL, apiKey string) *Client {
	return &Client{
		apiKey: apiKey,
		apiURL: apiURL,
	}
}

func (c *Client) request(path, method string, data interface{}, dst interface{}) error {
	url := fmt.Sprintf("%s/api/v1/%s", c.apiURL, path)
	var d []byte
	var err error
	if data != nil {
		d, err = json.Marshal(&data)
		if err != nil {
			return err
		}
	}
	req, err := http.NewRequest(method, url, bytes.NewReader(d))
	if err != nil {
		return err
	}
	req.Header.Add("Authorization", fmt.Sprintf("ExtraHop apikey=%s", c.apiKey))
	req.Header.Add("Content-Type", "application/json")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if dst == nil {
		return nil
	}
	if resp.StatusCode != 200 {
		b, err := ioutil.ReadAll(resp.Body)
		if err != nil {
			return err
		}
		return fmt.Errorf("Response Code %v: %s", resp.StatusCode, b)
	}
	return json.NewDecoder(resp.Body).Decode(dst)
}

func (c *Client) post(path string, data interface{}, dst interface{}) error {
	return c.request(path, "POST", data, dst)
}

// Posible Values for the Cycle Parameter of a Metric Query
var (
	CycleAuto  = "auto"
	Cycle30Sec = "30sec"
	Cycle5Min  = "5min"
	Cycle1Hr   = "1hr"
	Cycle24Hr  = "24hr"
)

//Metrics
type MetricQuery struct {
	// Can be"auto", "30sec", "5min", "1hr", "24hr"
	Cycle string `json:"cycle:`
	From  int64  `json:"from"`
	// Currently these seem secret, can be net or app though
	Category string `json:"metric_category"`
	//Stats.Values in the response becomes increases in length when there are more than one stat MetricSpec Requested
	Specs []MetricSpec `json:"metric_specs"`
	// OID becomes
	ObjectIds []int64 `json:"object_ids"`
	//Can be "network", "device", "application", "vlan", "device_group", and "activity_group".
	Type  string `json:"object_type"`
	Until int64  `json:"until"`
}

type KeyPair struct {
	Key1Regex        string `json:"key1,omitempty"`
	Key2Regex        string `json:"key2,omitempty"` // I can't find an example using 2 keys at the moment
	OpenTSDBKey1     string `json:"-"`
	Key2OpenTSDBKey2 string `json:"-"`
}

type MetricSpec struct {
	Name     string `json:"name"`
	CalcType string `json:"calc_type"`
	// The type of Stats.Values changes when there are keys added. It goes from []ints to an [][]structs, so the tag can be included
	KeyPair
	Percentiles []int64 `json:"percentiles,omitempty"`
	// The following are not part of the extrahop API
	OpenTSDBMetric string `json:"-"`
}

type MetricStat struct {
	Duration int64 `json:"duration"`
	Oid      int64 `json:"oid"`
	Time     int64 `json:"time"`
}

type MetricStatSimple struct {
	MetricStat
	Values []int64 `json:"values"`
}

type MetricStatKeyed struct {
	MetricStat
	Values [][]struct {
		Key struct {
			KeyType string `json:"key_type"`
			Str     string `json:"str"`
		} `json:"key"`
		Value int64  `json:"value"`
		Vtype string `json:"vtype"`
	} `json:"values"`
}

type MetricResponseBase struct {
	Cycle  string `json:"cycle"`
	From   int64  `json:"from"`
	NodeID int64  `json:"node_id"`
	Until  int64  `json:"until"`
}

type MetricResponseSimple struct {
	MetricResponseBase
	Stats []MetricStatSimple `json:"stats"`
}

type MetricResponseKeyed struct {
	MetricResponseBase
	Stats []MetricStatKeyed `json:"stats"`
}

func (mr *MetricResponseSimple) OpenTSDBDataPoints(metricNames []string, objectKey string, objectIdToName map[int64]string) (opentsdb.MultiDataPoint, error) {
	// Each position in Values should corespond to the order of
	// of Specs object. So len of Values == len(mq.Specs) I think.
	// Each item in Stats has a UID, which will map to the
	// requested object IDs
	var md opentsdb.MultiDataPoint
	for _, s := range mr.Stats {
		name, ok := objectIdToName[s.Oid]
		if !ok {
			return md, fmt.Errorf("no name found for oid %s", s.Oid)
		}
		time := s.Time
		if time < 1 {
			return md, fmt.Errorf("encountered a time less than 1")
		}
		for i, v := range s.Values {
			if len(metricNames) < i {
				return md, fmt.Errorf("no corresponding metric name at index %v", i)
			}
			metricName := metricNames[i]
			md = append(md, &opentsdb.DataPoint{
				Metric:    metricName,
				Timestamp: time / 1000,
				Tags:      opentsdb.TagSet{objectKey: name},
				Value:     v,
			})
		}
	}
	return md, nil
}

// Simple Metric query is for when you are making a query that doesn't
// have any facets ("Keys").
func (c *Client) SimpleMetricQuery(cycle, category, objectType string, fromMS, untilMS int64, metricsNames []string, objectIds []int64) (MetricResponseSimple, error) {
	mq := MetricQuery{
		Cycle:     cycle,
		Category:  category,
		ObjectIds: objectIds,
		Type:      objectType,
		From:      fromMS,
		Until:     untilMS,
	}
	for _, name := range metricsNames {
		mq.Specs = append(mq.Specs, MetricSpec{Name: name})
	}
	m := MetricResponseSimple{}
	err := c.post("metrics", &mq, &m)
	return m, err
}

// Keyed Metric query is for when you are making a query that has facets ("Keys"). For example bytes "By L7 Protocol"
func (mr *MetricResponseKeyed) OpenTSDBDataPoints(metrics []MetricSpec, objectKey string, objectIdToName map[int64]string) (opentsdb.MultiDataPoint, error) {
	// Only tested against one key, didn't find example with 2 keys yet
	var md opentsdb.MultiDataPoint
	for _, s := range mr.Stats {
		name, ok := objectIdToName[s.Oid]
		if !ok {
			return md, fmt.Errorf("no name found for oid %s", s.Oid)
		}
		time := s.Time
		if time < 1 {
			return md, fmt.Errorf("encountered a time less than 1")
		}
		for i, values := range s.Values {
			if len(metrics) < i {
				return md, fmt.Errorf("no corresponding metric name at index %v", i)
			}
			metricName := metrics[i].OpenTSDBMetric
			key1 := metrics[i].OpenTSDBKey1
			for _, v := range values {
				md = append(md, &opentsdb.DataPoint{
					Metric:    metricName,
					Timestamp: time / 1000,
					Tags:      opentsdb.TagSet{objectKey: name, key1: v.Key.Str},
					Value:     v.Value,
				})
			}
		}
	}
	return md, nil
}

func (c *Client) KeyedMetricQuery(cycle, category, objectType string, fromMS, untilMS int64, metrics []MetricSpec,
	objectIds []int64) (MetricResponseKeyed, error) {
	mq := MetricQuery{
		Cycle:     cycle,
		Category:  category,
		ObjectIds: objectIds,
		Type:      objectType,
		From:      fromMS,
		Until:     untilMS,
	}
	for _, spec := range metrics {
		mq.Specs = append(mq.Specs, spec)
	}
	m := MetricResponseKeyed{}
	err := c.post("metrics", &mq, &m)
	return m, err
}
