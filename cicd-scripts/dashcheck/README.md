
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

