package gohop

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"net/http"
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

type MetricSpec struct {
	Name     string `json:"name"`
	CalcType string `json:"calc_type"`
	// The type of Stats.Values changes when there are keys added. It goes from []ints to an [][]structs, so the tag can be included
	Key1        string  `json:"key1,omitempty"`
	Key2        string  `json:"key2,omitempty"` // I can't find an example using 2 keys at the moment
	Percentiles []int64 `json:"percentiles,omitempty"`
}

type MetricStat struct {
	Duration int `json:"duration"`
	Oid      int `json:"oid"`
	Time     int `json:"time"`
}

type MetricStatSimple struct {
	MetricStat
	Values []int `json:"values"`
}

type MetricStatKeyed struct {
	MetricStat
	Values [][]struct {
		Key struct {
			KeyType string `json:"key_type"`
			Str     string `json:"str"`
		} `json:"key"`
		Value int    `json:"value"`
		Vtype string `json:"vtype"`
	} `json:"values"`
}

type MetricResponseBase struct {
	Cycle  string `json:"cycle"`
	From   int    `json:"from"`
	NodeID int    `json:"node_id"`
	Until  int    `json:"until"`
}

type MetricResponseSimple struct {
	MetricResponseBase
	Stats []MetricStatSimple `json:"stats"`
}

type MetricResponseKeyed struct {
	MetricResponseBase
	Stats []MetricStatKeyed `json:"stats"`
}

func (c *Client) SimpleMetricQuery(cycle, category, objectType string, from, until int64, metricsNames []string, objectIds []int64) (*MetricResponseSimple, error) {
	mq := MetricQuery{
		Cycle:     cycle,
		Category:  category,
		ObjectIds: objectIds,
		Type:      objectType,
		From:      from,
		Until:     until,
	}
	for _, name := range metricsNames {
		mq.Specs = append(mq.Specs, MetricSpec{Name: name})
	}
	m := MetricResponseSimple{}
	err := c.post("metrics", &mq, &m)
	return &m, err
}
