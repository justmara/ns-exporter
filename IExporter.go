package main

import (
	"context"
)

type IExporter interface {
	LoadDeviceStatuses(queue chan NsEntry, limit int64, skip int64, ctx context.Context)
	LoadTreatments(queue chan NsTreatment, limit int64, skip int64, ctx context.Context)
	Close(ctx context.Context)
}
