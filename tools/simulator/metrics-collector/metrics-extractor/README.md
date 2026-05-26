# Note
This container captures the details of the timeseries data, a cluster will send to ACM Hub if it was brought under ACM's management. It does not matter if the cluster is really under ACM maagement as well. The data gathered is useful either way.

1. A docker image has been created for this container and is publicly available. It is at `quay.io/bjoydeep/metrics-extractor:latest`.
1. You are free to use it or build your own.
1. In the `docker run` command given below, a file called `timeseries.txt` will be created in the `/tmp` directory(mounted). This is the output or extract of the timeseries, the cluster would send to ACM Hub, if it was brought under ACM's management.
1. This output will help the ACM team to run simulation with `a certain` number of these clusters to figure out performance or just do a calculation without running any load.

### To build the docker image
docker build -t metrics-extractor .

### To use the docker image in quay.io
docker run -e OC_CLUSTER_URL=https://api.xyz.com:6443 -e OC_TOKEN=sha256~elK -v /tmp:/output quay.io/bjoydeep/metrics-extractor:latest

### To get into the image and debug
docker run -it metrics-extractor /bin/bash