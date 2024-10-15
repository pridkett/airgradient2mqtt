airgradient2mqtt
================

Patrick Wagstrom &lt;160672+pridkett@users.noreply.github.com&gt;

October 2024

Overview
--------

InfluxDB Setup
--------------

Although this isn't strictly necessary, you'll want to run some code in your InfluxDB server to establish retention rules and continuous queries to aggregate hourly and daily data.

```sql
CREATE RETENTION POLICY "six_months" ON "airgradient" DURATION 26w REPLICATION 1 DEFAULT;
CREATE RETENTION POLICY "hourly_aggregates" ON "airgradient" DURATION INF REPLICATION 1;
CREATE RETENTION POLICY "daily_aggregates" ON "airgradient" DURATION INF REPLICATION 1;

-- atmp  atmp_compensated nox_index nox_raw pm003_count pm01 pm02 pm02_compensated pm10 rco2 rhum rhum_compensated 
-- tvoc_index tvoc_raw wifi 

CREATE CONTINUOUS QUERY "cq_hourly_aggregates" ON "airgradient"
BEGIN
  SELECT
    MEAN(aqi) AS "mean_aqi", MIN(aqi) AS "min_aqi", MAX(aqi) AS "max_aqi",
    MEAN(atmp) AS "mean_atmp", MIN(atmp) AS "min_atmp", MAX(atmp) AS "max_atmp",
    MEAN(atmp_compensated) AS "mean_atmp_compensated", MIN(atmp_compensated) AS "min_atmp_compensated", MAX(atmp_compensated) AS "max_atmp_compensated",
    MEAN(nox_index) AS "mean_nox_index", MIN(nox_index) AS "min_nox_index", MAX(nox_index) AS "max_nox_index",
    MEAN(nox_raw) AS "mean_nox_raw", MIN(nox_raw) AS "mean_nox_raw", MAX(nox_raw) AS "max_nox_raw",
    MEAN(pm003_count) AS "mean_pm003_count", MIN(pm003_count) AS "min_pm003_count", MAX(pm003_count) AS "max_pm003_count",
    MEAN(pm01) AS "mean_pm01", MIN(pm01) AS "min_pm01", MAX(pm01) AS "max_pm01",
    MEAN(pm02) AS "mean_pm02", MIN(pm02) AS "min_pm02", MAX(pm02) AS "max_pm02",
    MEAN(pm02_compensated) AS "mean_pm02_compensated", MIN(pm02_compensated) AS "min_pm02_compensated", MAX(pm02_compensated) AS "max_pm02_compensated",
    MEAN(pm10) AS "mean_pm10", MIN(pm10) AS "min_pm10", MAX(pm10) AS "max_pm10",
    MEAN(rco2) AS "mean_rco2", MIN(rco2) AS "min_rco2", MAX(rco2) AS "max_rco2",
    MEAN(rhum) AS "mean_rhum", MIN(rhum) AS "min_rhum", MAX(rhum) AS "max_rhum",
    MEAN(rhum_compensated) AS "mean_rhum_compensated", MIN(rhum_compensated) AS "min_rhum_compensated", MAX(rhum_compensated) AS "max_rhum_compensated",
    MEAN(tvoc_index) AS "mean_tvoc_index", MIN(tvoc_index) AS "min_tvoc_index", MAX(tvoc_index) AS "max_tvoc_index",
    MEAN(tvoc_raw) AS "mean_tvoc_raw", MIN(tvoc_raw) AS "min_tvoc_raw", MAX(tvoc_raw) AS "max_tvoc_raw",
    MEAN(wifi) AS "mean_wifi", MIN(wifi) AS "min_wifi", MAX(wifi) AS "max_wifi"
  INTO "airgradient"."hourly_aggregates"."airgradient_hourly"
  FROM "airgradient"
  GROUP BY time(1h), *
END;

CREATE CONTINUOUS QUERY "cq_daily_aggregates" ON "airgradient"
BEGIN
  SELECT
    MEAN(aqi) AS "mean_aqi", MIN(aqi) AS "min_aqi", MAX(aqi) AS "max_aqi",
    MEAN(atmp) AS "mean_atmp", MIN(atmp) AS "min_atmp", MAX(atmp) AS "max_atmp",
    MEAN(atmp_compensated) AS "mean_atmp_compensated", MIN(atmp_compensated) AS "min_atmp_compensated", MAX(atmp_compensated) AS "max_atmp_compensated",
    MEAN(nox_index) AS "mean_nox_index", MIN(nox_index) AS "min_nox_index", MAX(nox_index) AS "max_nox_index",
    MEAN(nox_raw) AS "mean_nox_raw", MIN(nox_raw) AS "mean_nox_raw", MAX(nox_raw) AS "max_nox_raw",
    MEAN(pm003_count) AS "mean_pm003_count", MIN(pm003_count) AS "min_pm003_count", MAX(pm003_count) AS "max_pm003_count",
    MEAN(pm01) AS "mean_pm01", MIN(pm01) AS "min_pm01", MAX(pm01) AS "max_pm01",
    MEAN(pm02) AS "mean_pm02", MIN(pm02) AS "min_pm02", MAX(pm02) AS "max_pm02",
    MEAN(pm02_compensated) AS "mean_pm02_compensated", MIN(pm02_compensated) AS "min_pm02_compensated", MAX(pm02_compensated) AS "max_pm02_compensated",
    MEAN(pm10) AS "mean_pm10", MIN(pm10) AS "min_pm10", MAX(pm10) AS "max_pm10",
    MEAN(rco2) AS "mean_rco2", MIN(rco2) AS "min_rco2", MAX(rco2) AS "max_rco2",
    MEAN(rhum) AS "mean_rhum", MIN(rhum) AS "min_rhum", MAX(rhum) AS "max_rhum",
    MEAN(rhum_compensated) AS "mean_rhum_compensated", MIN(rhum_compensated) AS "min_rhum_compensated", MAX(rhum_compensated) AS "max_rhum_compensated",
    MEAN(tvoc_index) AS "mean_tvoc_index", MIN(tvoc_index) AS "min_tvoc_index", MAX(tvoc_index) AS "max_tvoc_index",
    MEAN(tvoc_raw) AS "mean_tvoc_raw", MIN(tvoc_raw) AS "min_tvoc_raw", MAX(tvoc_raw) AS "max_tvoc_raw",
    MEAN(wifi) AS "mean_wifi", MIN(wifi) AS "min_wifi", MAX(wifi) AS "max_wifi"
  INTO "airgradient"."daily_aggregates"."airgradient_daily"
  FROM "airgradient"
  GROUP BY time(1d), *
END;

```