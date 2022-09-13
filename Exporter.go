package main

import (
	"context"
)

type Exporter struct {
	client IExporter
}

func NewExporterFromMongo(uri string, db string, user string, ctx context.Context) *Exporter {
	exporter := &Exporter{
		client: NewMongoClient(uri, db, user, ctx),
	}
	return exporter
}

func NewExporterFromNS(uri string, token string, user string) *Exporter {
	exporter := &Exporter{
		client: NewNSClient(uri, token, user),
	}
	return exporter
}

func (worker Exporter) processClient(deviceStatuses chan NsEntry, treatments chan NsTreatment, limit int64, skip int64, ctx context.Context) {
	wg.Add(2)
	go worker.client.LoadDeviceStatuses(deviceStatuses, limit, skip, ctx)
	go worker.client.LoadTreatments(treatments, limit, skip, ctx)
}
