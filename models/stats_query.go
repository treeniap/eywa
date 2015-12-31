package models

import (
	"errors"
	"github.com/vivowares/octopus/Godeps/_workspace/src/gopkg.in/olivere/elastic.v3"
	. "github.com/vivowares/octopus/utils"
	"strconv"
	"strings"
	"time"
)

type StatsQuery struct {
	Channel   *Channel
	TimeStart time.Time
	TimeEnd   time.Time
}

func (q *StatsQuery) Parse(params map[string]string) error {
	if timeRange, found := params["time_range"]; !found {
		return errors.New("missing time_range for channel stats")
	} else {
		ranges := strings.Split(timeRange, ":")
		if len(ranges) != 2 || len(ranges[0]) == 0 {
			return errors.New("invalid time_range format: " + timeRange)
		}
		start, err := strconv.ParseInt(ranges[0], 10, 64)
		if err != nil {
			return errors.New("error parsing time_range: " + timeRange)
		}
		q.TimeStart = time.Unix(MilliSecToSec(start), MilliSecToNano(start)).UTC()

		if len(ranges[1]) > 0 {
			end, err := strconv.ParseInt(ranges[1], 10, 64)
			if err != nil {
				return errors.New("error parsing time_range: " + timeRange)
			}
			q.TimeEnd = time.Unix(MilliSecToSec(end), MilliSecToNano(end)).UTC()
		} else {
			q.TimeEnd = time.Now().UTC()
		}
	}

	if q.TimeStart.After(q.TimeEnd) {
		return errors.New("invalid time_range, start_time is later than end_time")
	}

	return nil
}

func (q *StatsQuery) QueryES() (interface{}, error) {
	filterAgg := elastic.NewFilterAggregation()

	boolQ := elastic.NewBoolQuery()
	rangeQ := elastic.NewRangeQuery("timestamp").
		From(NanoToMilli(q.TimeStart.UnixNano())).
		To(NanoToMilli(q.TimeEnd.UnixNano()))
	boolQ.Must(rangeQ)

	filterAgg.Filter(boolQ)

	for _, tag := range q.Channel.Tags {
		filterAgg.SubAggregation(tag, elastic.NewTermsAggregation().Field(tag).Size(0))
	}

	resp, err := IndexClient.Search().
		SearchType("count").
		Index(TimedIndices(q.Channel, q.TimeStart, q.TimeEnd)).
		Type(IndexType).
		Aggregation("name", filterAgg).
		Do()

	res := make(map[string]interface{})
	for _, tag := range q.Channel.Tags {
		res[tag] = []interface{}{}
	}

	if err != nil {
		return res, nil
	} else {
		if agg, found := resp.Aggregations.Filter("name"); found {
			for _, tag := range q.Channel.Tags {
				tagStats, found := agg.Aggregations.Terms(tag)
				if found {
					for _, bucket := range tagStats.Buckets {
						res[tag] = append(res[tag].([]interface{}), bucket.Key)
					}
				}
			}
			return res, nil
		}
		return nil, nil
	}
}