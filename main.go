package main

import (
	"errors"
	"flag"
	"log"
	"time"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/credentials"
	"github.com/aws/aws-sdk-go/aws/session"
	"github.com/aws/aws-sdk-go/service/cloudwatch"
	mp "github.com/mackerelio/go-mackerel-plugin-helper"
)

// SQSPlugin mackerel plugin
type SQSPlugin struct {
	AccessKeyID     string
	SecretAccessKey string
	CloudWatch      *cloudwatch.CloudWatch
	QueueName       string
	Prefix          string
	Region          string
}

type metric struct {
	Type         string
	Name         string
	Unit         string
	MackerelName string
}

func (m metric) name() string {
	if m.MackerelName != "" {
		return m.MackerelName
	}
	return m.Name
}

// GraphDefinition interface for mackerel plugin
func (sp SQSPlugin) GraphDefinition() map[string](mp.Graphs) {
	return map[string]mp.Graphs{
		"messages": mp.Graphs{
			Label: sp.QueueName + " Message",
			Unit:  "integer",
			Metrics: [](mp.Metrics){
				mp.Metrics{Name: "NumberOfMessagesSent", Label: "NumberOfMessagesSent"},
				mp.Metrics{Name: "NumberOfMessagesReceived", Label: "NumberOfMessagesReceived"},
				mp.Metrics{Name: "NumberOfMessagesDeleted", Label: "NumberOfMessagesDeleted"},
				mp.Metrics{Name: "NumberOfEmptyReceives", Label: "NumberOfEmptyReceives"},
			},
		},
		"message_size": mp.Graphs{
			Label: sp.QueueName + " Sent Message Size",
			Unit:  "bytes",
			Metrics: [](mp.Metrics){
				mp.Metrics{Name: "SentMessageSizeAverage", Label: "SentMessageSizeAvg"},
				mp.Metrics{Name: "SentMessageSizeMax", Label: "SentMessageSizeMax"},
				mp.Metrics{Name: "SentMessageSizeMin", Label: "SentMessageSizeMin"},
			},
		},
		"queue": mp.Graphs{
			Label: sp.QueueName + " Approximate Message",
			Unit:  "integer",
			Metrics: [](mp.Metrics){
				mp.Metrics{Name: "ApproximateNumberOfMessagesDelayed", Label: "ApproximateNumberOfMessagesDelayed"},
				mp.Metrics{Name: "ApproximateNumberOfMessagesVisible", Label: "ApproximateNumberOfMessagesVisible"},
				mp.Metrics{Name: "ApproximateNumberOfMessagesNotVisible", Label: "ApproximateNumberOfMessagesNotVisible"},
				mp.Metrics{Name: "ApproximateAgeOfOldestMessage", Label: "ApproximateAgeOfOldestMessage"},
			},
		},
	}
}

// MetricKeyPrefix interface for mackerel plugin
func (sp SQSPlugin) MetricKeyPrefix() string {
	if sp.Prefix == "" {
		return "sqs." + sp.QueueName
	}
	return sp.Prefix
}

// FetchMetrics interface for mackerel plugin
func (sp SQSPlugin) FetchMetrics() (map[string]interface{}, error) {

	stats := make(map[string]interface{})

	metrics := []metric{
		{
			Name: "NumberOfMessagesSent",
			Type: "Sum",
			Unit: "Count",
		},
		{
			Name: "NumberOfMessagesReceived",
			Type: "Sum",
			Unit: "Count",
		},
		{
			Name: "NumberOfEmptyReceives",
			Type: "Sum",
			Unit: "Count",
		},
		{
			Name: "NumberOfMessagesDeleted",
			Type: "Sum",
			Unit: "Count",
		},
		{
			Name:         "SentMessageSize",
			Type:         "Average",
			Unit:         "Bytes",
			MackerelName: "SentMessageSizeAverage",
		},
		{
			Name:         "SentMessageSize",
			Type:         "Maximum",
			Unit:         "Bytes",
			MackerelName: "SentMessageSizeMax",
		},
		{
			Name:         "SentMessageSize",
			Type:         "Minimum",
			Unit:         "Bytes",
			MackerelName: "SentMessageSizeMin",
		},
		{
			Name: "ApproximateNumberOfMessagesDelayed",
			Type: "Average",
			Unit: "Count",
		},
		{
			Name: "ApproximateNumberOfMessagesVisible",
			Type: "Average",
			Unit: "Count",
		},
		{
			Name: "ApproximateNumberOfMessagesNotVisible",
			Type: "Average",
			Unit: "Count",
		},
		{
			Name: "ApproximateAgeOfOldestMessage",
			Type: "Maximum",
			Unit: "Seconds",
		},
	}

	for _, metric := range metrics {
		metricName := metric.name()
		val, err := sp.getLastPoint(metric)
		if err != nil {
			log.Printf("%s: %s", metricName, err)
		}
		stats[metricName] = val
	}

	return stats, nil
}

func (sp SQSPlugin) getLastPoint(metric metric) (float64, error) {

	now := time.Now()

	response, err := sp.CloudWatch.GetMetricStatistics(&cloudwatch.GetMetricStatisticsInput{
		Dimensions: []*cloudwatch.Dimension{
			{
				Name:  aws.String("QueueName"),
				Value: aws.String(sp.QueueName),
			},
		},
		StartTime:  aws.Time(now.Add(time.Duration(300) * time.Second * -1)), // 5 min (to fetch at least 1 data-point)
		EndTime:    aws.Time(now),
		Period:     aws.Int64(60),
		Namespace:  aws.String("AWS/SQS"),
		MetricName: aws.String(metric.Name),
		Unit:       aws.String(metric.Unit),
		Statistics: []*string{aws.String(metric.Type)},
	})
	if err != nil {
		return 0, err
	}

	datapoints := response.Datapoints
	if len(datapoints) == 0 {
		return 0, errors.New("fetched no datapoints")
	}

	least := time.Now()
	var latestVal float64
	for _, dp := range datapoints {
		if dp.Timestamp.Before(least) {
			least = *dp.Timestamp
			switch metric.Type {
			case "Average":
				latestVal = *dp.Average
			case "Sum":
				latestVal = *dp.Sum
			case "Maximum":
				latestVal = *dp.Maximum
			}
		}
	}

	return latestVal, nil
}

func (sp *SQSPlugin) prepare() error {
	sess, err := session.NewSession()
	if err != nil {
		return err
	}

	config := aws.NewConfig()
	if sp.AccessKeyID != "" && sp.SecretAccessKey != "" {
		config = config.WithCredentials(credentials.NewStaticCredentials(sp.AccessKeyID, sp.SecretAccessKey, ""))
	}
	config = config.WithRegion(sp.Region)
	sp.CloudWatch = cloudwatch.New(sess, config)

	return nil
}

func main() {
	var (
		tmpfile   string
		prefix    string
		secret    string
		accessKey string
		queueName string
		region    string
	)

	flag.StringVar(&tmpfile, "tempfile", "", "tmpfile")
	flag.StringVar(&secret, "secret-access-key", "", "AWS Secret Access key")
	flag.StringVar(&accessKey, "access-key-id", "", "AWS Access Key ID")
	flag.StringVar(&queueName, "queue-name", "", "SQS Queue Name")
	flag.StringVar(&region, "region", "us-east-1", "AWS Region")
	flag.StringVar(&prefix, "metric-key-prefix", "", "metric key prefix")
	flag.Parse()

	p := SQSPlugin{
		SecretAccessKey: secret,
		AccessKeyID:     accessKey,
		QueueName:       queueName,
		Prefix:          prefix,
		Region:          region,
	}

	if err := p.prepare(); err != nil {
		log.Fatalln(err)
	}

	helper := mp.NewMackerelPlugin(p)
	helper.Tempfile = tmpfile

	helper.Run()
}
