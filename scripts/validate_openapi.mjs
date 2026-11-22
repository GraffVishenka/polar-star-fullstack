#!/usr/bin/env node
// Проверка OpenAPI-контракта без сети (Б13): синтаксис YAML, разрешимость
// всех $ref, уникальность operationId, наличие ответов и security-схем.
//
// Запуск (нужен js-yaml; он уже есть в зависимостях фронтенда):
//   cd frontend && node ../scripts/validate_openapi.mjs
// или
//   node scripts/validate_openapi.mjs   # если js-yaml доступен из корня
//
// Почему не только redocly: redocly тянется через npx (сеть), а этот скрипт
// работает в офлайн-CI и на машине без Node-пакетов в корне.

import { createRequire } from "node:module";
import { readFileSync, readdirSync } from "node:fs";
import { dirname, resolve } from "node:path";
import { fileURLToPath } from "node:url";

const here = dirname(fileURLToPath(import.meta.url));
const repoRoot = resolve(here, "..");
const specPath = process.env.OPENAPI_PATH || resolve(repoRoot, "openapi/openapi.yaml");

// js-yaml лежит в frontend/node_modules (pnpm хранит транзитивные зависимости в
// .pnpm и не поднимает их в корень) либо в корневом node_modules.
function loadYAML() {
  // Ищем анкеры в самом репозитории и выше: при работе в git worktree
  // (`.worktrees/be-2`) node_modules обычно лежит в основном чекауте.
  const anchors = [];
  for (let dir = repoRoot, i = 0; i < 5; i += 1, dir = resolve(dir, "..")) {
    anchors.push(resolve(dir, "frontend/package.json"));
    anchors.push(resolve(dir, "package.json"));
  }
  for (const anchor of anchors) {
    try {
      return createRequire(anchor)("js-yaml");
    } catch {
      // пробуем следующий способ
    }
  }
  // pnpm: frontend/node_modules/.pnpm/js-yaml@<version>/node_modules/js-yaml
  for (let dir = repoRoot, i = 0; i < 5; i += 1, dir = resolve(dir, "..")) {
    try {
      const store = resolve(dir, "frontend/node_modules/.pnpm");
      const entry = readdirSync(store).find((name) => name.startsWith("js-yaml@"));
      if (entry) {
        return createRequire(resolve(store, entry, "node_modules/js-yaml/package.json"))("js-yaml");
      }
    } catch {
      // нет pnpm-store на этом уровне
    }
  }
  throw new Error(
    "js-yaml не найден: выполните `cd frontend && pnpm install` или `npm install js-yaml`",
  );
}

const yaml = loadYAML();
const errors = [];
const fail = (message) => errors.push(message);

let doc;
try {
  doc = yaml.load(readFileSync(specPath, "utf8"));
} catch (err) {
  console.error(`openapi: не удалось разобрать ${specPath}: ${err.message}`);
  process.exit(1);
}
if (!doc || typeof doc !== "object") {
  console.error("openapi: документ пуст или не является объектом");
  process.exit(1);
}

const version = String(doc.openapi || "");
if (!/^3\.\d+\.\d+$/.test(version)) {
  fail(`openapi: ожидается версия 3.x.y, получено ${JSON.stringify(doc.openapi)}`);
}

// --- Разрешение $ref ---------------------------------------------------------
function resolvePointer(pointer) {
  if (!pointer.startsWith("#/")) return { ok: false, reason: "внешние $ref не поддерживаются" };
  let node = doc;
  for (const raw of pointer.slice(2).split("/")) {
    const part = raw.replace(/~1/g, "/").replace(/~0/g, "~");
    if (node === null || typeof node !== "object" || !(part in node)) {
      return { ok: false, reason: `сегмент ${part} не найден` };
    }
    node = node[part];
  }
  return { ok: true, node };
}

const refs = new Map();
function walk(value, where) {
  if (Array.isArray(value)) {
    value.forEach((item, index) => walk(item, `${where}[${index}]`));
    return;
  }
  if (!value || typeof value !== "object") return;
  if (typeof value.$ref === "string") {
    const result = resolvePointer(value.$ref);
    if (!result.ok) fail(`${where}: $ref ${value.$ref} — ${result.reason}`);
    refs.set(value.$ref, (refs.get(value.$ref) || 0) + 1);
  }
  for (const [key, nested] of Object.entries(value)) {
    if (key === "$ref") continue;
    walk(nested, `${where}.${key}`);
  }
}
walk(doc, "#");

// --- Пути и операции --------------------------------------------------------
const methods = new Set(["get", "put", "post", "delete", "options", "head", "patch", "trace"]);
const paths = doc.paths;
if (!paths || typeof paths !== "object" || Object.keys(paths).length === 0) {
  fail("openapi: paths пуст");
}
let operations = 0;
let notImplemented = 0;
const operationIDs = new Set();
for (const [path, item] of Object.entries(paths || {})) {
  if (typeof path !== "string" || !path.startsWith("/")) {
    fail(`paths: ключ ${JSON.stringify(path)} должен начинаться с /`);
  }
  if (!item || typeof item !== "object") {
    fail(`paths ${path}: пустой path item`);
    continue;
  }
  for (const [method, op] of Object.entries(item)) {
    if (!methods.has(method)) continue;
    operations += 1;
    const where = `${method.toUpperCase()} ${path}`;
    if (!op || typeof op !== "object") {
      fail(`${where}: операция пуста`);
      continue;
    }
    if (typeof op.operationId !== "string" || op.operationId.trim() === "") {
      fail(`${where}: нет operationId`);
    } else if (operationIDs.has(op.operationId)) {
      fail(`${where}: повторный operationId ${op.operationId}`);
    } else {
      operationIDs.add(op.operationId);
    }
    if (!op.responses || typeof op.responses !== "object" || Object.keys(op.responses).length === 0) {
      fail(`${where}: нет responses`);
    }
    if (op.responses && op.responses["501"]) notImplemented += 1;
    if (op.security) {
      if (!Array.isArray(op.security)) {
        fail(`${where}: security должен быть массивом`);
      } else {
        for (const requirement of op.security) {
          for (const scheme of Object.keys(requirement || {})) {
            if (!doc.components?.securitySchemes?.[scheme]) {
              fail(`${where}: security-схема ${scheme} не описана`);
            }
          }
        }
      }
    }
    // Параметры и тело: все $ref уже проверены обходом, но path-параметры
    // должны быть описаны, иначе операция невалидна. Параметр может быть
    // задан через $ref (IdPath и т.п.) — имя достаём после разрешения.
    const params = [...(item.parameters || []), ...(op.parameters || [])];
    const declared = new Set();
    for (const param of params) {
      let resolved = param;
      if (param && typeof param.$ref === "string") {
        const pointer = resolvePointer(param.$ref);
        resolved = pointer.ok ? pointer.node : null;
      }
      if (resolved && typeof resolved.name === "string") declared.add(resolved.name);
    }
    for (const match of path.matchAll(/\{([^}]+)\}/g)) {
      if (!declared.has(match[1])) {
        fail(`${where}: path-параметр ${match[1]} не описан в parameters`);
      }
    }
  }
}

// --- Итог ------------------------------------------------------------------
if (errors.length > 0) {
  console.error(`openapi: найдено ошибок — ${errors.length}`);
  for (const message of errors) console.error(`  - ${message}`);
  process.exit(1);
}
console.log(
  `openapi: OK — ${Object.keys(paths).length} путей, ${operations} операций, ` +
    `${refs.size} уникальных $ref, заглушек 501: ${notImplemented}`,
);
