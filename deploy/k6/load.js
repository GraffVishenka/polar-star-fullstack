// k6-профиль нагрузки для Б13 (PLAN §7, критерий: 200 RPS, p95 ≤ 300 мс).
//
// Сценарий — чтение основных экранов под ADMIN-сессией: список заявок,
// оборудование, дерево БЗ, личная статистика, публичный дашборд. Запись не
// нагружаем: она меняет данные и мешает повторяемости замеров.
//
// Подготовка стенда (10 000 заявок) — deploy/k6/README.md:
//   make demo
//   docker compose exec -T mysql mysql -upolar -ppolar polar_star < deploy/k6/seed-load.sql
//
// Запуск:
//   BASE_URL=http://localhost:8080 ADMIN_LOGIN=demo.admin1 ADMIN_PASSWORD=Demo_12345 \
//     k6 run deploy/k6/load.js
//
// Порог: http_req_failed < 1%, p95 < 300 мс. Профиль: 50 → 200 RPS за 5 минут,
// затем 5 минут на 200 RPS (HOLD_DURATION=0s — только разгон).

import http from "k6/http";
import { check } from "k6";

const BASE_URL = (__ENV.BASE_URL || "http://localhost:8080").replace(/\/$/, "");
const ADMIN_LOGIN = __ENV.ADMIN_LOGIN || "demo.admin1";
const ADMIN_PASSWORD = __ENV.ADMIN_PASSWORD || "Demo_12345";
const HOLD = __ENV.HOLD_DURATION || "5m";

export const options = {
  discardResponseBodies: true,
  scenarios: {
    reads: {
      executor: "ramping-arrival-rate",
      startRate: 50,
      timeUnit: "1s",
      stages: [
        { target: 50, duration: "1m" },
        { target: 200, duration: "4m" },
        { target: 200, duration: HOLD },
      ],
      preAllocatedVUs: 200,
      maxVUs: 600,
      gracefulStop: "30s",
    },
  },
  thresholds: {
    // Ошибки 4xx считаются неуспехом: токен должен жить весь тест (15 минут).
    http_req_failed: ["rate<0.01"],
    http_req_duration: ["p(95)<300"],
    checks: ["rate>0.99"],
  },
};

// setup один раз логинится и возвращает токен; VU его переиспользуют.
export function setup() {
  const response = http.post(
    `${BASE_URL}/api/auth/login`,
    JSON.stringify({ login: ADMIN_LOGIN, password: ADMIN_PASSWORD }),
    { headers: { "Content-Type": "application/json" }, tags: { name: "login" } },
  );
  if (response.status !== 200) {
    throw new Error(`login: статус ${response.status}: ${response.body}`);
  }
  const token = response.json("access_token");
  if (!token) throw new Error("login: в ответе нет access_token");
  return { token };
}

// Наборы параметров для статистики: текущий месяц и последние 30 дней.
function periodParams() {
  const now = new Date();
  const iso = (date) => date.toISOString().slice(0, 10);
  const from = new Date(now);
  from.setUTCDate(from.getUTCDate() - 30);
  return `from=${iso(from)}&to=${iso(now)}`;
}

export default function (data) {
  const headers = { Authorization: `Bearer ${data.token}` };
  const period = periodParams();

  // Ротация по сценариям: 40% — заявки, по 15% — остальные экраны.
  const bucket = __ITER % 20;
  let request;
  if (bucket < 8) {
    const page = bucket < 4 ? 1 : 2;
    request = { name: "tickets", url: `${BASE_URL}/api/tickets?page=${page}&size=20` };
  } else if (bucket < 11) {
    request = { name: "equipment", url: `${BASE_URL}/api/equipment?page=1&size=20` };
  } else if (bucket < 14) {
    request = { name: "kb_tree", url: `${BASE_URL}/api/kb/tree` };
  } else if (bucket < 17) {
    request = { name: "stats_me", url: `${BASE_URL}/api/stats/me?${period}` };
  } else if (bucket < 19) {
    request = { name: "public_settings", url: `${BASE_URL}/api/public/settings` };
  } else {
    request = { name: "public_tickets", url: `${BASE_URL}/api/public/tickets?page=1&size=20` };
  }

  const response = http.get(request.url, { headers, tags: { name: request.name } });
  check(response, {
    "статус 200": (r) => r.status === 200,
    "есть тело": (r) => r.body === null || r.body.length > 0,
  });
}
