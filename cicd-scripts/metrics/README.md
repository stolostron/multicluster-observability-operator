# Metrics CI tools

Collection of simple CI tools and scripts to validate the list of metrics collected for a list of dashboards.
This helps maintain the list of metrics and rules in sync with the dashboards, and ensure that we don't collect
more metrics than needed.

More precisely, it helps verifying that:
* Metrics needed in dashboards are collected, and not more
* Rules needed in dashboards are defined, and not more
* Query rules are valid

## Usage

When adding a new list of dashboards in a new directory, make sure that you define the corresponding scrapeConfig and rules.
Then add a target in the Makefile to run the metrics check for the new dashboards, following the existing examples.
