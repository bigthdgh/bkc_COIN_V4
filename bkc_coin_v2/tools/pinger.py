#!/usr/bin/env python3
import os
import time
import urllib.request


def parse_urls(raw: str):
    out = []
    for item in (raw or "").split(","):
        u = item.strip()
        if not u:
            continue
        out.append(u)
    return out


def main():
    urls = parse_urls(os.getenv("PING_URLS", ""))
    if not urls:
        print("PING_URLS is empty. Example:")
        print("PING_URLS=https://node1.onrender.com/api/v1/health,https://node2.onrender.com/api/v1/health")
        return

    try:
        interval = int(os.getenv("PING_INTERVAL_SEC", "600"))
    except ValueError:
        interval = 600
    if interval < 30:
        interval = 30

    timeout = 10
    print(f"Starting pinger: {len(urls)} urls, interval={interval}s")

    while True:
        started = time.strftime("%Y-%m-%d %H:%M:%S")
        for url in urls:
            try:
                req = urllib.request.Request(url, method="GET")
                with urllib.request.urlopen(req, timeout=timeout) as resp:
                    code = resp.getcode()
                print(f"[{started}] {url} -> {code}")
            except Exception as exc:
                print(f"[{started}] {url} -> ERROR: {exc}")
        time.sleep(interval)


if __name__ == "__main__":
    main()
