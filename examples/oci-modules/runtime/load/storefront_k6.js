import http from "k6/http";
import { sleep, check } from "k6";

export const options = {
  vus: 10,
  duration: "1m",
  thresholds: {
    http_req_failed: ["rate<0.01"],
    http_req_duration: ["p(95)<500"],
  },
};

const baseUrl = __ENV.BASE_URL || "http://payments-api:8080";

export default function () {
  const response = http.get(baseUrl + "/catalog");
  check(response, {
    "status is 200": (r) => r.status === 200,
  });
  sleep(1);
}
