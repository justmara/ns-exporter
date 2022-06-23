package main

import (
	"context"
	"flag"
	"fmt"
	influxdb2 "github.com/influxdata/influxdb-client-go/v2"
	"github.com/influxdata/influxdb-client-go/v2/api/write"
	"github.com/peterbourgon/ff/v3"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/bson/bsontype"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
	"log"
	"os"
	"regexp"
	"strconv"
	"sync"
	"time"
)

var wg sync.WaitGroup

func main() {
	fs := flag.NewFlagSet("ns-exporter", flag.ContinueOnError)
	var (
		mongoUri    = fs.String("mongo-uri", "", "Mongo-db uri to download from")
		mongoDb     = fs.String("mongo-db", "", "Mongo-db database name")
		limit       = fs.Int64("limit", 0, "number of records to read from mongo-db")
		skip        = fs.Int64("skip", 0, "number of records to skip from mongo-db")
		influxUri   = fs.String("influx-uri", "", "InfluxDb uri to download from")
		influxToken = fs.String("influx-token", "", "InfluxDb access token")
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
	influx := make(chan write.Point)

	wg.Add(2)

	go parseDeviceStatuses(mongodb, influx, limit, skip, ctx)
	go parseTreatments(mongodb, influx, limit, skip, ctx)

	var wgInflux = &sync.WaitGroup{}
	go func() {
		wgInflux.Add(1)
		defer wgInflux.Done()
		var count = 0
		writeAPI := influxdb2.NewClient(*influxUri, *influxToken).WriteAPIBlocking("ns", "ns")

		for point := range influx {

			if len(point.FieldList()) == 0 && len(point.TagList()) == 0 {

				fmt.Println("empty point for time: ", point.Time(), " of type: ", point.Name())
				continue
			}

			err = writeAPI.WritePoint(ctx, &point)
			count++
			if err != nil {

				fmt.Println("error writing: ", point.Time(), ", name: ", point.Name())
				log.Fatal(err)
			}
		}

		fmt.Println("total writen: ", count)

	}()

	wg.Wait()
	close(influx)
	wgInflux.Wait()
}

func parseDeviceStatuses(mongo *mongo.Database, influx chan write.Point, limit *int64, skip *int64, ctx context.Context) {
	defer wg.Done()

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

	var count = 0
	var lastbg = 0.0
	var lasttick float64 = 0
	for cur.Next(ctx) {
		var entry NsEntry
		err := cur.Decode(&entry)
		if err != nil {
			fmt.Println(cur.Current.String())
			log.Fatal(err)
		}

		point := influxdb2.NewPointWithMeasurement("openaps").
			AddField("iob", entry.OpenAps.IOB.IOB).
			AddField("basal_iob", entry.OpenAps.IOB.BasalIOB).
			AddField("activity", entry.OpenAps.IOB.Activity).
			SetTime(entry.OpenAps.IOB.Time)
		if entry.OpenAps.Suggested.Bg > 0 {
			field := cur.Current.Lookup("openaps", "suggested", "tick")

			var tick float64 = 0
			if field.Type == bsontype.String {
				tick, err = strconv.ParseFloat(field.StringValue(), 32)
			}
			if field.Type == bsontype.Int32 {
				tick = float64(field.AsInt64())
			}

			if err != nil {
				log.Fatal(err)
			}
			if lastbg == entry.OpenAps.Suggested.Bg &&
				lasttick == tick &&
				tick != 0.0 {
				// deduplication, because nightscout still allows duplicate records to be added
				fmt.Println("skipping duplicate bg record: ", entry.OpenAps.IOB.Time, ", bg: ", entry.OpenAps.Suggested.Bg, ", tick: ", tick)
				continue
			}

			lastbg = entry.OpenAps.Suggested.Bg
			lasttick = tick
			point.
				AddField("bg", entry.OpenAps.Suggested.Bg).
				AddField("tick", tick).
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

		count++
		influx <- *point

		fmt.Println("time: ", entry.OpenAps.IOB.Time, "iob:", entry.OpenAps.IOB.IOB, ", bg: ", entry.OpenAps.Suggested.Bg)
	}
	fmt.Println("total sent devicestatuses: ", count)
	if err := cur.Err(); err != nil {
		log.Fatal(err)
	}
}

func parseTreatments(mongo *mongo.Database, influx chan write.Point, limit *int64, skip *int64, ctx context.Context) {
	defer wg.Done()

	var noted = map[string]bool{
		"Site Change":         true,
		"Insulin Change":      true,
		"Pump Battery Change": true,
		"Sensor Change":       true,
		"Sensor Start":        true,
		"Sensor Stop":         true,
		"BG Check":            true,
		"Exercise":            true,
		"Announcement":        true,
		"Question":            true,
		//"Note": true,
		"OpenAPS Offline": true,
		"D.A.D. Alert":    true,
		"Mbg":             true,
		//"Carb Correction": true,
		//"Bolus Wizard": true,
		//"Correction Bolus": true,
		//"Meal Bolus": true,
		//"Combo Bolus": true,
		//"Temporary Target": true,
		//"Temporary Target Cancel": true,
		//"Profile Switch": true,
		//"Snack Bolus": true,
		//"Temp Basal": true,
		//"Temp Basal Start": true,
		//"Temp Basal End": true,
	}

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

	var count = 0
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
		} else if entry.EventType == "Temporary Target" {
			point.
				AddField("duration", entry.Duration).
				AddField("target_top", entry.TargetTop).
				AddField("target_bottom", entry.TargetBottom).
				AddField("units", entry.Units).
				AddField("reason", entry.Reason).
				AddTag(tagName, "tt")
		} else if len(entry.Notes) > 0 {
			point.AddField("notes", entry.Notes)
		} else if noted[entry.EventType] {
			point.AddField("notes", entry.EventType)
		}

		count++
		influx <- *point
		fmt.Println("time: ", point.Time(), ", type: ", entry.EventType)
	}

	fmt.Println("total sent treatments: ", count)
	if err := cur.Err(); err != nil {
		log.Fatal(err)
	}
}
