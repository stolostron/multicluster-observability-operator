# Persistent Stores used in Open Cluster Management Observability

Open Cluster Management Observability is a stateful application. It creates following persistent volumes (there are more than 1 copies as it runs as stateful sets).

### List of Stateful sets

| Name | Purpose |
| ----------- | ----------- |
| alertmanager | Alertmanager stores its silencing, nflog etc in its storage. nflog is append-only log of active/resolved notifications along with the notified receiver, and a hash digest of the notification's identifying contents|
| thanos-compact | The compactor needs local disk space to store intermediate data for its processing as well as bucket state cache. Generally, for medium sized bucket about 100GB should be enough to keep working as the compacted time ranges grow over time. However, this highly depends on size of the blocks. In worst case scenario compactor has to have space adequate to 2 times 2 weeks (if your maximum compaction level is 2 weeks) worth of smaller blocks to perform compaction. First, to download all of those source blocks, second to build on disk output of 2 week block composed of those smaller ones. On-disk data is safe to delete between restarts and should be the first attempt to get crash-looping compactors unstuck. However, it’s recommended to give the Compactor persistent disk in order to effectively use bucket state cache between restarts. |
| thanos-rule | The thanos ruler evaluates Prometheus recording and alerting rules against chosen query API via repeated query. Rule results are written back to disk in the Prometheus 2.0 storage format. |
| thanos-receive-default | Thanos receiver accepts incoming data (Prometheus remote-write requests) and writes these into a local instance of the Prometheus TSDB. Periodically (every 2 hours) TSDB blocks are uploaded to the Object storage for long term storage and compaction. The amount of hours/days of data retained in this stateful set (acts a local cache) was fixed in ACM 2.1/2.2. It has been exposed as an API parameter in ACM 2.3: _RetentionInLocal_ |
| thanos-store-shard| It acts primarily as an API gateway and therefore does not need significant amounts of local disk space. It joins a Thanos cluster on startup and advertises the data it can access. It keeps a small amount of information about all remote blocks on local disk and keeps it in sync with the bucket. This data is generally safe to delete across restarts at the cost of increased startup times. |
| thanos-store-memcached | Thanos Store Gateway supports an index cache using Memached to speed up postings and series lookups from TSDB blocks indexes. |
| | |




### Configuring the Stateful sets

In ACM 2.1 and ACM 2.2 only the following was exposed using the Mutlicluster Observability Operator API - _observability.open-cluster-management.io/v1beta1_

```
    //defaults shown below
    statefulSetSize: 10Gi
    statefulSetStorageClass: gp2
```
In ACM 2.3, taking into consideration that one size fits all results in wasting space, we have allowed all settings can be individually tweaked in the enhanced API - _observability.open-cluster-management.io/v1beta2_

```
    //defaults shown below
    StorageClass: gp2
	AlertmanagerStorageSize: 1Gi 
	RuleStorageSize: 1Gi
	CompactStorageSize: 100 Gi
	ReceiveStorageSize: 100 Gi
	StoreStorageSize: 10 Gi

```
Do note that the default storage class, as configured in the system, is used for configuring the persistent volumes automatically unless a different storage class is specified in the Custom Resource specification. If no storage class exists - for example in default OpenShift bare metal installtions, it will need to be created or the installation of ACM Observability will not succeed.

### Object Store
In addition to the persistent volumes above, the time series historical data is stored in Object stores. Thanos uses object storage as primary storage for metrics and metadata related to them. Details of Object storage and downsampling will be covered in another document.