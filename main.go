package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"github.com/gobeam/stringy"
	"github.com/rs/zerolog"
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
	var originalPositions, updatedPositions Positions

	pflag.String("time.format", "2006-01-02", "time field layout in golang time.Parse format")
	pflag.String("time.field", "time", "field to get the time for")
	pflag.String("out", "", "File to write to")
	pflag.String("positions.file", "positions.yml", "file to keep track of positions")
	pflag.Bool("debug", false, "more logging")
	pflag.String("default.label", "table", "label to set from key if there are no labels in record")
	getConfig()

	log := SetupLog()
	var err error
	originalPositions, err = LoadPositions()
	if err != nil {
		log.Fatal().Err(err).Msg("couldn't load positions")
	}
	updatedPositions = originalPositions
	if viper.GetString("out") == "" {
		fmt.Println("No output file provided")
		os.Exit(1)
	}

	outputFile, err := os.Create(viper.GetString("out"))
	if err != nil {
		log.Fatal().Err(err).Msg("failed to create output")
	}
	defer outputFile.Close()

	for _, f := range pflag.Args() {
		processFile(log, f, outputFile, &positions)
	}
	_, err = fmt.Fprintln(outputFile, "# EOF")
	if err != nil {
		log.Fatal().Err(err).Msg("couldn't write to file")
	}

	err = SavePositions(updatedPositions)
	if err != nil {
		log.Fatal().Err(err).Msg("couldn't save positions")
	}

}

func processFile(log zerolog.Logger, inputFileName string, outputFile *os.File, originalPositions Positions, updatedPositions *Positions) {
	jsonFile, err := os.Open(inputFileName)
	if err != nil {
		log.Fatal().Err(err).Msg("failed to open input")
	}
	defer jsonFile.Close()

	basename := filepath.Base(inputFileName)

	i := strings.IndexByte(basename, '_')
	j := strings.Index(basename, "Z-")

	if i == -1 {
		log.Fatal().Err(err).Str("file", basename).Msg("filename does not have an _")
		return
	}
	if j == -1 {
		log.Fatal().Err(err).Str("file", basename).Msg("filename does not have a timestamp")
		return
	}

	defaultTimeStampStr := basename[i+1 : j+1]
	log.Debug().Str("default_ts", defaultTimeStampStr).Msg("Default Timestamp")
	defaultTimeStamp, err := time.Parse("20060102T150405Z", defaultTimeStampStr)
	if err != nil {
		log.Debug().Err(err).Msg("failed to parse default time stamp, defaulting to run time")
		defaultTimeStamp = runTimeStamp
	}

	basename = basename[:i]
	baseMetricName := to_snake_case(basename)

	filterTS, found := originalPositions.Positions[baseMetricName]
	if !found {
		filterTS = time.Time{}
	}
	maxTs := time.Time{}
	filteredMetrics := 0

	byteValue, _ := ioutil.ReadAll(jsonFile)
	//	log.V(1).Info("Starting to process", "in", inputFileName, "out", outputFileName, "metric", baseMetricName)

	var jsonMap []map[string]interface{}
	json.Unmarshal(byteValue, &jsonMap)

	log.Trace().Int("count", len(jsonMap)).Msg("Read Json lines")

	//	prevTimeStamp, _ := time.Parse("2006-01-02", "2999-01-01")
	metrics := make(map[string][]string)

	timefield := viper.GetString("time.field")
	//	timeformat := viper.GetString("time.format")
	timeformats := viper.GetStringSlice("time.formats")
	for _, row := range jsonMap {
		rowTime, found := row[timefield].(string)
		var rowTimeStamp time.Time
		if found {
			succesfullyParsed := false
			for _, timeformat := range timeformats {
				rowTimeStamp, err = time.Parse(timeformat, rowTime)
				if err == nil {
					succesfullyParsed = true
					break
				}
			}
			if !succesfullyParsed {
				log.Error().Err(err).Str("timefield", rowTime).Str("file", basename).Msg("couldn't parse time")
			}

		} else {
			rowTimeStamp = defaultTimeStamp
			log.Trace().Time("runtime", runTimeStamp).Msg("defaulted time")
		}

		// Filter metrics
		if filterTS.After(rowTimeStamp) || filterTS.Equal(rowTimeStamp) {
			filteredMetrics++
			continue
		}
		if maxTs.Before(rowTimeStamp) {
			maxTs = rowTimeStamp
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
						log.Error().Err(err).Msg("Failed to convert count_percent to float")
					}
					pc := strings.Trim(fields[1], "(%)")
					values["pc"], err = strconv.ParseFloat(pc, 64)
				} else {
					log.Error().Err(errors.New("not enough fields")).Msg("couldn't parse count_percent field")
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
	log.Info().Int("num", filteredMetrics).Msg("Filtered metrics")

	for _, m := range metrics {
		for _, v := range m {
			_, err = fmt.Fprintln(outputFile, v)
			if err != nil {
				log.Error().Err(err).Msg("couldn't write to file")
			}
		}
		if len(m) > 0 {
			log.Trace().
				Int("count", len(m)).
				Str("sample", m[0]).
				Str("input", inputFileName).
				Msg("Wrote Open Metrics")
		}
	}
	updatedPos, _ := updatedPositions.Positions[baseMetricName]
	if maxTs.After(updatedPos) {
		updatedPositions.Positions[baseMetricName] = maxTs
	}
}

func to_snake_case(basename string) string {
	baseMetricName := stringy.New(basename).SnakeCase("?", "").ToLower()
	return baseMetricName
}

func addMetrics(metriclist *map[string][]string, values map[string]float64, labels map[string]string, t time.Time, mn string) {
	labelsList := ""
	defaultMetricNames := viper.GetStringMapString("default_labelname")
	commonLabelNames := viper.GetStringSlice("common_label_names")
	defaultLabelName := defaultMetricNames[mn]
	numberOfCommonLabels := 0
	for k, v := range labels {
		if labelsList != "" {
			labelsList = labelsList + fmt.Sprintf(",%s=\"%s\"", to_snake_case(k), v)
		} else {
			labelsList = fmt.Sprintf("%s=\"%s\"", to_snake_case(k), v)
		}
		if contains(commonLabelNames, k) {
			numberOfCommonLabels++
		}
	}
	onlyCommonLabels := numberOfCommonLabels == len(labels)

	for k, v := range values {

		_, found := (*metriclist)[k]
		if !found {
			(*metriclist)[k] = make([]string, 0)
		}
		var metricString string
		if labelsList == "" {
			defaultLabel := ""   // normally no labels are added
			if len(values) > 1 { // except when there are multiple values in a record, where we add the key name with default label
				//				defaultLabel = viper.GetString("default.label") + "=\"" + k + "\""
				defaultLabel = defaultLabelName + "=\"" + k + "\""
			}
			metricString = fmt.Sprintf("%s{%s} %f %d", mn, defaultLabel, v, t.Unix())
		} else {
			if len(values) == 1 {
				metricString = fmt.Sprintf("%s_%s{%s} %f %d", mn, k, labelsList, v, t.Unix())
			} else {
				if onlyCommonLabels {
					defaultLabel := defaultLabelName + "=\"" + k + "\""
					metricString = fmt.Sprintf("%s{%s,%s} %f %d", mn, defaultLabel, labelsList, v, t.Unix())
				} else {
					metricString = fmt.Sprintf("%s_%s{%s} %f %d", mn, k, labelsList, v, t.Unix())
				}
			}
		}
		(*metriclist)[k] = append((*metriclist)[k], metricString)
		if viper.GetBool("debug") {
			fmt.Println(metricString)
		}
	}
}

func contains(s []string, str string) bool {
	for _, v := range s {
		if v == str {
			return true
		}
	}

	return false
}
