package main

import (
	"encoding/json"
	"fmt"
	"github.com/go-kit/log"
	"github.com/go-kit/log/level"
	"github.com/prometheus/common/promlog"
	"github.com/prometheus/common/promlog/flag"
	"gopkg.in/alecthomas/kingpin.v2"
	"io/ioutil"
	"os"
	"time"
)

var (
	inputFile  = kingpin.Flag("in", "json input file.").Required().String()
	outputFile = kingpin.Flag("out", "open metrics output file").Required().String()
	metricName = kingpin.Flag("base", "base name for the metrics").Required().String()
)

func main() {
	promlogConfig := &promlog.Config{}
	flag.AddFlags(kingpin.CommandLine, promlogConfig)

	kingpin.Version("json_to_openm 0.1.0")
	kingpin.HelpFlag.Short('h')
	kingpin.Parse()
	logger := promlog.New(promlogConfig)
	level.Info(logger).Log("input", *inputFile, *outputFile)
	jsonFile, err := os.Open(*inputFile)
	if err != nil {
		level.Error(logger).Log("msg", "failed to open input", "err", err)
	}
	defer jsonFile.Close()
	outputFile, err := os.Create(*outputFile)
	if err != nil {
		level.Error(logger).Log("msg", "failed to create output", "err", err)
	}
	defer outputFile.Close()
	byteValue, _ := ioutil.ReadAll(jsonFile)

	var jsonMap []map[string]interface{}
	json.Unmarshal(byteValue, &jsonMap)

	//	prevTimeStamp, _ := time.Parse("2006-01-02", "2999-01-01")
	metrics := make(map[string][]interface{})

	for _, row := range jsonMap {
		t, err := time.Parse("2006-01-02", row["time"].(string))
		if err != nil {
			level.Error(logger).Log("msg", "couldn't parse time", "err", err)
		}
		/*
			for fillIn := prevTimeStamp; fillIn.Before(t); fillIn = fillIn.Add(time.Minute * 5) {
				metrics["low"] = append(metrics["low"], getMetric(err, "low", row.Low, fillIn, logger))
				if err != nil {
					level.Error(logger).Log("msg", "couldn't write to file", "err", err)
				}
			} */
		for k, v := range row {
			if k != "time" {
				_, found := metrics[k]
				if !found {
					metrics[k] = make([]interface{}, 0)
				}
				metricRow := getMetric(err, k, v.(float64), t, logger)
				metrics[k] = append(metrics[k], metricRow)
			}

		}

		//		prevTimeStamp = t

	}

	fmt.Print(metrics)
	for _, m := range metrics {
		for _, v := range m {
			_, err = fmt.Fprintln(outputFile, v)
			if err != nil {
				level.Error(logger).Log("msg", "couldn't write to file", "err", err)
			}

		}
	}

	_, err = fmt.Fprintln(outputFile, "# EOF")
	if err != nil {
		level.Error(logger).Log("msg", "couldn't write to file", "err", err)
	}

}

func getMetric(err error, metric string, value float64, t time.Time, logger log.Logger) string {
	return fmt.Sprintf("%s_%s %f %d", *metricName, metric, value, t.Unix())
}
