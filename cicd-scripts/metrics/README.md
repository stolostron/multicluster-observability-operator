# Metrics CI

Objective is to ensure that:
* Metrics needed in dashboards are collected 
* Rules needed in dashboards are defined
* No unneeded metric is collected
* No unneeded rule is defined
* Metrics matchers are valid in scrapeconfigs => 
* Query rules are valid => Use promtool check rules

E2E tests must ensure that core platform metrics are collected and that dashboards are populated with data (some queries might be invalid)


Generate metrics count stats:

```bash
./extract-dashboards-metrics.sh | tr '\n' ' ' | xargs ./count-metrics.sh > metrics-stats.txt
```

Sort extracted metrics to identify highest cardinality ones:

```bash
sort -k2,2nr metrics-stats.txt | grep -v " 0"
```


Check prom rules
```bash
cat grafana/nexus/acm/prometheus-rule.yaml | yq '.spec' | promtool check rules
```

