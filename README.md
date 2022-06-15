# ns-exporter
Nightscout exporter to InfluxDB

usage variants:
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
	limit        - number of records to read from MongoDb
	skip         - number of records to skip from MongoDb
	influx-uri   - InfluxDb uri to download from
	influx-token - InfluxDb access token

arguments also can be provided via env with `NS_EXPORTER_` prefix:

	NS_EXPORTER_MONGO_URI=
	NS_EXPORTER_MONGO_DB=
	NS_EXPORTER_LIMIT=
	NS_EXPORTER_SKIP=
	NS_EXPORTER_INFLUX_URI=
	NS_EXPORTER_INFLUX_TOKEN=
