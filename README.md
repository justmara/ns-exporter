# ns-exporter
Nightscout exporter to InfluxDB

### usage variants:
1. inline
```
go build
./ns-exporter
```
2. docker
```
docker build -t ns-exporter .
docker run -d ns-exporter:latest
```

arguments:

	mongo-uri    - MongoDb uri to download from
	mongo-db     - MongoDb database name
	ns-uri       - Nightscout server url to download from
	ns-token     - Nigthscout server API Authorization Token
	limit        - number of records to read from MongoDb
	skip         - number of records to skip from MongoDb
	influx-uri   - InfluxDb uri to download from
	influx-token - InfluxDb access token

arguments also can be provided via env with `NS_EXPORTER_` prefix:

	NS_EXPORTER_MONGO_URI=
	NS_EXPORTER_MONGO_DB=
	NS_EXPORTER_NS_URI=
	NS_EXPORTER_NS_TOKEN=
	NS_EXPORTER_LIMIT=
	NS_EXPORTER_SKIP=
	NS_EXPORTER_INFLUX_URI=
	NS_EXPORTER_INFLUX_TOKEN=

So you can choose the data source: direct MongoDB or Nightscout REST API. Supplying required set of parameters will trigger related consumer.
You can even supply both and get from both sources :)

### Presentation

I'm using Grafana dashboard for viewing data. To setup grafana with InfluxDB you need to follow InfluxDB's [instructions](https://docs.influxdata.com/influxdb/v2.3/tools/grafana/).
The sample dashboard can be imported from `grafana.json`. It uses both InfluxQL and Flux datasources for different panels. Some can be omitted, some can be reworker based on other InfluxDB datasource query type. 
Anyway they're provided as samples, for educational purpose :)