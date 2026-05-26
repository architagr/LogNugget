// k6 load test for zerolog vs LogNugget Loki benchmark.
//
// Run against zerolog-server:
//   k6 run -e TARGET_URL=http://localhost:8080 k6.js
//
// Run against lognugget-server:
//   k6 run -e TARGET_URL=http://localhost:8081 k6.js
//
// Summary metrics captured: http_req_duration (p50/p95/p99/p999), rps, errors.
// B3 multi-header propagation: X-B3-TraceId, X-B3-SpanId, X-B3-Sampled.

import http from "k6/http";
import { check } from "k6";
import { Counter, Trend } from "k6/metrics";

const errCount = new Counter("errors");
const latency = new Trend("handler_latency_us", true);

export const options = {
  scenarios: {
    constant_load: {
      executor: "constant-arrival-rate",
      rate: 10000,          // 10,000 req/s
      timeUnit: "1s",
      duration: "60s",
      preAllocatedVUs: 500,
      maxVUs: 2000,
    },
  },
  thresholds: {
    http_req_failed: ["rate<0.001"],          // < 0.1% errors
    http_req_duration: ["p(99)<500"],         // p99 < 500 ms
  },
};

const TARGET = __ENV.TARGET_URL || "http://localhost:8080";

// Generate a random 16-char hex string (64-bit span ID).
function randHex16() {
  return Math.floor(Math.random() * 0xffffffff).toString(16).padStart(8, "0") +
         Math.floor(Math.random() * 0xffffffff).toString(16).padStart(8, "0");
}

// Generate a random 32-char hex string (128-bit trace ID).
function randHex32() {
  return randHex16() + randHex16();
}

export default function () {
  const params = {
    timeout: "10s",
    headers: {
      "X-B3-TraceId": randHex32(),
      "X-B3-SpanId":  randHex16(),
      "X-B3-Sampled": "1",
    },
  };

  const res = http.get(`${TARGET}/api/v1/work`, params);

  const ok = check(res, {
    "status 200": (r) => r.status === 200,
  });

  if (!ok) {
    errCount.add(1);
  } else {
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
=== ${target} @ ${TARGET} (OTel B3, 10 000 req/s) ===
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
