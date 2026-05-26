// k6 load test for zerolog vs LogNugget Loki benchmark.
//
// Run against zerolog-server:
//   k6 run -e TARGET_URL=http://localhost:8080 k6.js
//
// Run against lognugget-server:
//   k6 run -e TARGET_URL=http://localhost:8081 k6.js
//
// Summary metrics captured: http_req_duration (p50/p95/p99/p999), rps, errors.

import http from "k6/http";
import { check, sleep } from "k6";
import { Counter, Trend } from "k6/metrics";

const errCount = new Counter("errors");
const latency = new Trend("handler_latency_us", true);

export const options = {
  scenarios: {
    constant_load: {
      executor: "constant-arrival-rate",
      rate: 1000,           // 1000 req/s
      timeUnit: "1s",
      duration: "60s",
      preAllocatedVUs: 200,
      maxVUs: 400,
    },
  },
  thresholds: {
    http_req_failed: ["rate<0.001"],          // < 0.1% errors
    http_req_duration: ["p(99)<200"],         // p99 < 200 ms (generous upper bound)
  },
};

const TARGET = __ENV.TARGET_URL || "http://localhost:8080";

export default function () {
  const res = http.get(`${TARGET}/api/v1/work`, { timeout: "10s" });

  const ok = check(res, {
    "status 200": (r) => r.status === 200,
  });

  if (!ok) {
    errCount.add(1);
  } else {
    // Parse latency_us from JSON body {"status":"ok","latency_us":N}
    try {
      const body = JSON.parse(res.body);
      if (body.latency_us !== undefined) {
        latency.add(body.latency_us);
      }
    } catch (_) {}
  }
}

export function handleSummary(data) {
  const d = data.metrics.http_req_duration;
  const target = TARGET.includes("8080") ? "zerolog" : "lognugget";

  return {
    stdout: `
=== ${target} @ ${TARGET} ===
requests:         ${data.metrics.http_reqs.values.count}
rps (avg):        ${data.metrics.http_reqs.values.rate.toFixed(1)}
http p50:         ${d.values["p(50)"].toFixed(2)} ms
http p95:         ${d.values["p(95)"].toFixed(2)} ms
http p99:         ${d.values["p(99)"].toFixed(2)} ms
http p99.9:       ${d.values["p(99.9)"].toFixed(2)} ms
errors:           ${data.metrics.errors ? data.metrics.errors.values.count : 0}
`,
  };
}
