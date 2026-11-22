#!/usr/bin/env python3
"""Печатает аннотации GitHub Actions для упавших Go-тестов.

Зачем: логи job'а недоступны без токена, а check-run annotations — публичны.
Скрипт читает вывод `go test -json` и для каждого упавшего теста печатает
`::error::<последние строки вывода>`, чтобы причина была видна через API.
"""

import json
import sys

path = sys.argv[1] if len(sys.argv) > 1 else "/tmp/tests.json"

fails = set()
outputs = {}

with open(path, encoding="utf-8", errors="replace") as handle:
    for line in handle:
        try:
            event = json.loads(line)
        except json.JSONDecodeError:
            continue
        key = (event.get("Package"), event.get("Test"))
        if event.get("Action") == "fail" and event.get("Test"):
            fails.add(key)
        elif event.get("Action") == "output" and event.get("Test"):
            outputs.setdefault(key, []).append(event.get("Output", ""))

if not fails:
    # Пакетный сбой без конкретного теста: показываем хвост вывода пакета.
    for (package, test), lines in outputs.items():
        if test:
            continue
        for text in lines[-10:]:
            print("::error::" + text.replace("\n", " ")[:900])

for key in sorted(fails, key=str):
    package, test = key
    print(f"::error::FAIL {package} :: {test}")
    for text in outputs.get(key, [])[-15:]:
        print("::error::" + text.replace("\n", " ")[:900])
