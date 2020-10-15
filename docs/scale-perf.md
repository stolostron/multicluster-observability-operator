
# Performance and scalability

This doc includes certain scalability and performance data of observability and you can use this information to help you plan your environment. 
The resource consumption later is not for single pod/deployment, but for the OpenShift project where observability components are installed. It's a sum value for all observability components.


*Note:* Data is based on the results from a lab environment at the time of testing.
Your results might vary, depending on your environment, network speed, and changes to the product.

## Test environment
In the test environment, hub and managed clusters are located in Amazon Web Services cloud platfrom and have the same topology/configuration as below:


Node | Flavor	| vCPU	| RAM (GiB)	| Disk type	| Disk size(GiB)/IOS	| Count	| Region
---  | ------ | ----  | --------- | --------- | ------------------  | ----- | ------ 
Master | m5.4xlarge | 16 |64 | gp2 | 100 | 3 | sa-east-1
Worker | m5.4xlarge | 16 |64 | gp2 | 100 | 3 | sa-east-1

For the observability deployment, it uses the "High" for availabilityConfig, which means for each kubernetes deployment has 2 instances and each statefulset has 3 instances.

During the test, different number of managed clusters will be simulated to push metrics and each test will last for 24 hours.
Throughput for each managed cluster is as below:

Pods | Interval(minute) | Time series per min
---- | ---------------- | -------------------
400 | 1 | 83000


## CPU
During test, CPU usage keeps stable
Size | CPU Usage(millicores)
---- | --------------
10 clusters | 400
20 clusters | 800

## Memory
Memory Usage RSS is from the metrics container_memory_rss. It keeps stabl during test.
Memory Usage Working Set is from the metrics container_memory_working_set_bytes. It increases along with the test. Value below is one after 24 hours.
Size | Memory Usage RSS(GiB) | Memory Usage Working Set(GiB)
---- | --------------------- | -----------------------------
10 clusters | 9.84 | 4.83
20 clusters | 13.10 | 8.76

## Persistent volume for thanos-receive component
Except thanos-receivee components, other components don't use much disk. Because metrics will be stored in thanos-receive until rentation time reached(retention time of thanos-receive is 4 days), 
Disk usage increases along with the test. Data below points out the disk usage after 1 day, so the final disk usage should multiply 4.
Size | Disk Usage(GiB)
---- | --------------
10 clusters | 2
20 clusters | 3

## Network transfer
During test, network transfer keeps stable
Size | Inbound Network Transfer(MiB per second) | Outbound Network Transfer(MiB per second)
---- | ---------------------------------------- | -----------------------------------------
10 clusters | 6.55 | 5.80
20 clusters | 13.08 | 10.9

## S3 storage
Total usage in S3 side increases along with the test. The metrics data will be kept in S3 until retention time reached(Default rentation time is 5 days).
Size | Total Usage(GiB)
---- | --------------
10 clusters | 16.2
20 clusters | 23.8
