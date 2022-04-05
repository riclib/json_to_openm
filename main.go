package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"github.com/go-logr/logr"
	"github.com/gobeam/stringy"
	"github.com/spf13/pflag"
	"github.com/spf13/viper"
	"io/ioutil"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

var (
	runTimeStamp = time.Now()
)

func main() {

	pflag.String("time.format", "2006-01-02", "time field layout in golang time.Parse format")
	pflag.String("time.field", "time", "field to get the time for")
	pflag.String("out", "", "File to write to")
	pflag.Bool("debug", false, "more logging")
	pflag.String("default.label", "table", "label to set from key if there are no labels in record")
	getConfig()

	log := SetupLog()

	if viper.GetString("out") == "" {
		fmt.Println("No output file provided")
		os.Exit(1)
	}

	outputFile, err := os.Create(viper.GetString("out"))
	if err != nil {
		log.Error(err, "failed to create output")
	}
	defer outputFile.Close()

	for _, f := range pflag.Args() {
		processFile(log, f, outputFile)
	}
	_, err = fmt.Fprintln(outputFile, "# EOF")
	if err != nil {
		log.Error(err, "couldn't write to file")
	}
}

func processFile(log logr.Logger, inputFileName string, outputFile *os.File) {
	jsonFile, err := os.Open(inputFileName)
	if err != nil {
		log.Error(err, "failed to open input")
	}
	defer jsonFile.Close()
	basename := filepath.Base(inputFileName)
	basename = basename[:strings.IndexByte(basename, '_')]
	baseMetricName := stringy.New(basename).SnakeCase("?", "").ToLower()

	byteValue, _ := ioutil.ReadAll(jsonFile)
	//	log.V(1).Info("Starting to process", "in", inputFileName, "out", outputFileName, "metric", baseMetricName)

	var jsonMap []map[string]interface{}
	json.Unmarshal(byteValue, &jsonMap)

	log.V(2).Info("Read json lines", "count", len(jsonMap))

	//	prevTimeStamp, _ := time.Parse("2006-01-02", "2999-01-01")
	metrics := make(map[string][]string)

	timefield := viper.GetString("time.field")
	timeformat := viper.GetString("time.format")
	for _, row := range jsonMap {
		rowTime, found := row[timefield].(string)
		var rowTimeStamp time.Time
		if found {
			rowTimeStamp, err = time.Parse(timeformat, rowTime)
			if err != nil {
				log.Error(err, "couldn't parse time")
			}
		} else {
			rowTimeStamp = runTimeStamp
			log.V(2).Info("defaulted time", "runtime", runTimeStamp)
		}

		values := make(map[string]float64)
		labels := make(map[string]string)

		for k, v := range row {
			switch k {
			case "time":
				// time handled above
			case "count_percent":
				// handle "315 (15.7%)"
				fields := strings.Fields(v.(string))
				if len(fields) == 2 {
					values["count"], err = strconv.ParseFloat(fields[0], 64)
					if err != nil {
						log.Error(err, "Failed to convert count_percent to float")
					}
					pc := strings.Trim(fields[1], "(%)")
					values["pc"], err = strconv.ParseFloat(pc, 64)
				} else {
					log.Error(errors.New("not enough fields"), "couldn't parse count_percent field")
				}
			default:
				switch v.(type) {
				case float64:
					values[k] = v.(float64)
				case string:
					labels[k] = v.(string)
				}
			}
		}

		addMetrics(&metrics, values, labels, rowTimeStamp, baseMetricName)
	}

	for _, m := range metrics {
		for _, v := range m {
			_, err = fmt.Fprintln(outputFile, v)
			if err != nil {
				log.Error(err, "couldn't write to file")
			}
		}
		if len(m) > 0 {
			log.Info("Wrote Open Metrics", "count", len(m), "sample", m[0], "input", inputFileName)
		}
	}

}

func addMetrics(metriclist *map[string][]string, values map[string]float64, labels map[string]string, t time.Time, mn string) {
	labelsList := ""
	for k, v := range labels {
		if labelsList != "" {
			labelsList = labelsList + fmt.Sprintf(", %s=\"%s\"", k, v)
		} else {
			labelsList = fmt.Sprintf("%s=\"%s\"", k, v)

		}
	}
	for k, v := range values {

		_, found := (*metriclist)[k]
		if !found {
			(*metriclist)[k] = make([]string, 0)
		}
		var metricString string
		if labelsList == "" {
			defaultLabel := viper.GetString("default.label") + "=\"" + k + "\""
			metricString = fmt.Sprintf("%s{%s} %f %d", mn, defaultLabel, v, t.Unix())
		} else {
			metricString = fmt.Sprintf("%s_%s{%s} %f %d", mn, k, labelsList, v, t.Unix())
		}
		(*metriclist)[k] = append((*metriclist)[k], metricString)
		if viper.GetBool("debug") {
			fmt.Println(metricString)
		}
	}
}
