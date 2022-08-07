package main

import (
	"context"
	"fmt"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/bson/bsontype"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
	"log"
	"strconv"
	"time"
)

type MongoClient struct {
	mongoUri string
	mongoDb  string
	db       *mongo.Database
	client   *mongo.Client
}

func NewMongoClient(uri string, db string, ctx context.Context) *MongoClient {
	c := &MongoClient{
		mongoUri: uri,
		mongoDb:  db,
	}

	client, err := mongo.NewClient(options.Client().ApplyURI(c.mongoUri))
	if err != nil {
		log.Fatal(err)
	}
	c.client = client

	err = client.Connect(ctx)
	if err != nil {
		log.Fatal(err)
	}

	err = client.Ping(ctx, nil)
	if err != nil {
		log.Fatal(err)
	}

	c.db = client.Database(c.mongoDb)
	return c
}

func (c MongoClient) LoadDeviceStatuses(queue chan NsEntry, limit int64, skip int64, ctx context.Context) {
	defer wg.Done()
	defer close(queue)

	collection := c.db.Collection("devicestatus")
	filter := bson.D{{"openaps", bson.D{{"$exists", true}}}}

	opts := options.Find()
	opts.SetSort(bson.D{{"created_at", -1}})
	if limit > 0 {
		opts.SetLimit(limit)
	}
	if skip > 0 {
		opts.SetSkip(skip)
	}

	cur, err := collection.Find(ctx, filter, opts)
	if err != nil {
		log.Fatal(err)
	}
	defer cur.Close(ctx)

	var count = 0
	for cur.Next(ctx) {
		var entry NsEntry
		err := cur.Decode(&entry)
		if err != nil {
			fmt.Println(cur.Current.String())
			log.Fatal(err)
		}
		if entry.OpenAps.Suggested.Bg > 0 {
			field := cur.Current.Lookup("openaps", "suggested", "tick")
			var tick float64 = 0
			if field.Type == bsontype.String {
				tick, err = strconv.ParseFloat(field.StringValue(), 32)
			}
			if field.Type == bsontype.Int32 {
				tick = float64(field.AsInt64())
			}
			entry.OpenAps.Suggested.Tick = tick
		}

		queue <- entry

		count++

		fmt.Println("time: ", entry.OpenAps.IOB.Time, "iob:", entry.OpenAps.IOB.IOB, ", bg: ", entry.OpenAps.Suggested.Bg)
	}
	fmt.Println("total devicestatuses sent: ", count)
	if err := cur.Err(); err != nil {
		log.Fatal(err)
	}
}

func (c MongoClient) LoadTreatments(queue chan NsTreatment, limit int64, skip int64, ctx context.Context) {
	defer wg.Done()
	defer close(queue)

	collection := c.db.Collection("treatments")
	filter := bson.D{}

	opts := options.Find()
	opts.SetSort(bson.D{{"created_at", -1}})
	if limit > 0 {
		opts.SetLimit(limit)
	}
	if skip > 0 {
		opts.SetSkip(skip)
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

		entry.CreatedAt = ptime

		queue <- entry
		count++

		fmt.Println("time: ", entry.CreatedAt, ", type: ", entry.EventType)
	}

	fmt.Println("total treatments sent: ", count)
	if err := cur.Err(); err != nil {
		log.Fatal(err)
	}
}

func (c MongoClient) Close(ctx context.Context) {
	c.client.Disconnect(ctx)
}
