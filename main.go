package main

import (
	"context"
	"flag"
	"fmt"
	influxdb2 "github.com/influxdata/influxdb-client-go/v2"
	"github.com/influxdata/influxdb-client-go/v2/api"
	"github.com/peterbourgon/ff/v3"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
	"log"
	"os"
	"regexp"
	"strconv"
	"time"
)

func main() {
	fs := flag.NewFlagSet("ns-exporter", flag.ContinueOnError)
	var (
		mongoUri    = fs.String("mongo-uri", "", "Mongo-db uri to download from")
		mongoDb     = fs.String("mongo-db", "", "Mongo-db database name")
		limit       = fs.Int64("limit", 0, "number of records to read from mongo-db")
		skip        = fs.Int64("skip", 0, "number of records to skip from mongo-db")
		influxUri   = fs.String("influx-uri", "", "InfluxDb uri to download from")
		influxToken = fs.String("influx-token", "", "InfluxDb uri to download from")
	)
	if err := ff.Parse(fs, os.Args[1:], ff.WithEnvVarPrefix("NS_EXPORTER")); err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		os.Exit(1)
	}

	ctx := context.Background()

	client, err := mongo.NewClient(options.Client().ApplyURI(*mongoUri))
	if err != nil {
		log.Fatal(err)
	}
	defer func() {
		if err = client.Disconnect(ctx); err != nil {
			panic(err)
		}
	}()

	err = client.Connect(ctx)
	if err != nil {
		log.Fatal(err)
	}

	err = client.Ping(ctx, nil)
	if err != nil {
		log.Fatal(err)
	}

	mongodb := client.Database(*mongoDb)

	influx := influxdb2.NewClient(*influxUri, *influxToken)
	writeAPI := influx.WriteAPIBlocking("ns", "ns")
	ParseDeviceStatuses(mongodb, writeAPI, limit, skip, ctx)
	ParseTreatments(mongodb, writeAPI, limit, skip, ctx)
}

func ParseDeviceStatuses(mongo *mongo.Database, influx api.WriteAPIBlocking, limit *int64, skip *int64, ctx context.Context) {

	reg := regexp.MustCompile("Dev: (?P<dev>[-0-9.]+),.*ISF: (?P<isf>[-0-9.]+),.*CR: (?P<cr>[-0-9.]+)")

	collection := mongo.Collection("devicestatus")
	filter := bson.D{{"openaps", bson.D{{"$exists", true}}}}

	opts := options.Find()
	opts.SetSort(bson.D{{"created_at", -1}})
	if *limit > 0 {
		opts.SetLimit(*limit)
	}
	if *skip > 0 {
		opts.SetSkip(*skip)
	}

	cur, err := collection.Find(ctx, filter, opts)
	if err != nil {
		log.Fatal(err)
	}
	defer cur.Close(ctx)

	for cur.Next(ctx) {
		var entry NsEntry
		err := cur.Decode(&entry)
		if err != nil {
			log.Fatal(err)
		}

		point := influxdb2.NewPointWithMeasurement("openaps").
			AddField("iob", entry.OpenAps.IOB.IOB).
			AddField("basal_iob", entry.OpenAps.IOB.BasalIOB).
			AddField("activity", entry.OpenAps.IOB.Activity).
			SetTime(entry.OpenAps.IOB.Time)
		if entry.OpenAps.Suggested.Bg > 0 {
			point.
				AddField("bg", entry.OpenAps.Suggested.Bg).
				AddField("eventual_bg", entry.OpenAps.Suggested.EventualBG).
				AddField("target_bg", entry.OpenAps.Suggested.TargetBG).
				AddField("insulin_req", entry.OpenAps.Suggested.InsulinReq).
				AddField("cob", entry.OpenAps.Suggested.COB).
				AddField("bolus", entry.OpenAps.Suggested.Units).
				AddField("tbs_rate", entry.OpenAps.Suggested.Rate).
				AddField("tbs_duration", entry.OpenAps.Suggested.Duration)
			if len(entry.OpenAps.Suggested.PredBGs.COB) > 0 {
				point.AddField("pred_cob", entry.OpenAps.Suggested.PredBGs.COB[len(entry.OpenAps.Suggested.PredBGs.COB)-1])
			}
			if len(entry.OpenAps.Suggested.PredBGs.IOB) > 0 {
				point.AddField("pred_iob", entry.OpenAps.Suggested.PredBGs.IOB[len(entry.OpenAps.Suggested.PredBGs.IOB)-1])
			}
			if len(entry.OpenAps.Suggested.PredBGs.UAM) > 0 {
				point.AddField("pred_uam", entry.OpenAps.Suggested.PredBGs.UAM[len(entry.OpenAps.Suggested.PredBGs.UAM)-1])
			}
			if len(entry.OpenAps.Suggested.PredBGs.ZT) > 0 {
				point.AddField("pred_zt", entry.OpenAps.Suggested.PredBGs.ZT[len(entry.OpenAps.Suggested.PredBGs.ZT)-1])
			}
			if len(entry.OpenAps.Suggested.Reason) > 0 {
				matches := reg.FindStringSubmatch(entry.OpenAps.Suggested.Reason)
				names := reg.SubexpNames()
				for i, match := range matches {
					if i != 0 {
						if len(match) > 0 {
							if rvalue, err := strconv.ParseFloat(match, 32); err == nil {
								point.AddField(names[i], rvalue)
							}
						}
					}
				}
			}
		}

		err = influx.WritePoint(ctx, point)
		if err != nil {
			log.Fatal(err)
		}

		fmt.Println("time: ", entry.OpenAps.IOB.Time, ", bg: ", entry.OpenAps.Suggested.Bg)
	}
	if err := cur.Err(); err != nil {
		log.Fatal(err)
	}
}

func ParseTreatments(mongo *mongo.Database, influx api.WriteAPIBlocking, limit *int64, skip *int64, ctx context.Context) {
	collection := mongo.Collection("treatments")
	filter := bson.D{}

	opts := options.Find()
	opts.SetSort(bson.D{{"created_at", -1}})
	if *limit > 0 {
		opts.SetLimit(*limit)
	}
	if *skip > 0 {
		opts.SetSkip(*skip)
	}

	cur, err := collection.Find(ctx, filter, opts)
	if err != nil {
		log.Fatal(err)
	}
	defer cur.Close(ctx)

	for cur.Next(ctx) {
		var entry NsTreatment
		err := cur.Decode(&entry)
		if err != nil {
			log.Fatal(err)
		}

		strtime := cur.Current.Lookup("created_at").StringValue()
		ptime, err := time.Parse(time.RFC3339, strtime)
		if err != nil {
			log.Fatal(err)
		}

		point := influxdb2.NewPointWithMeasurement("treatments").
			SetTime(ptime)
		tagName := "type"
		if entry.Carbs > 0 {
			point.
				AddField("carbs", entry.Carbs).
				AddTag(tagName, "carbs")
		}
		if entry.Insulin > 0 {
			point.
				AddField("bolus", entry.Insulin).
				AddTag(tagName, "bolus").
				AddTag("smb", strconv.FormatBool(entry.IsSMB))
		}
		if entry.EventType == "Temp Basal" {
			point.
				AddField("duration", entry.Duration).
				AddField("percent", entry.Percent).
				AddField("rate", entry.Rate).
				AddTag(tagName, "tbs")
		}

		if len(entry.Notes) > 0 {
			point.AddField("notes", entry.Notes)
		}

		err = influx.WritePoint(ctx, point)
		if err != nil {
			log.Fatal(err)
		}

		fmt.Println("time: ", point.Time(), ", type: ", entry.EventType)
	}

	if err := cur.Err(); err != nil {
		log.Fatal(err)
	}
}
