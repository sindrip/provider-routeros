#!/usr/bin/env python3
"""Probe disputed field types against a live CHR (hack/chr/run.sh).

For each curated field below — the type_mismatches bucket from audit.py,
with fixtures hand-picked per path — PATCH candidate values and record what
the router accepts and what it stores afterwards. The stored value catches
coercion: RouterOS's REST layer accepts JSON-style true/false for any
bool-backed type and maps them to that field's display labels (inherit/no,
long/short, yes/auto), while the labels themselves are the only other
accepted spellings.

Settings singletons reject PATCH; writes go through POST <path>/set (the
console tree encodes which menus are singletons: set without add).

Output (stdout) is the pinned config/type-verdicts.json. Entries where every
stored value is "?" could not be read back on CHR virtual hardware and are
inconclusive (e.g. sfp-rate-select on a virtio NIC).
"""
import base64
import json
import sys
import urllib.error
import urllib.request

BASE = "http://127.0.0.1:18080/rest"
AUTH = base64.b64encode(b"admin:").decode()


def rest(method, path, body=None):
    req = urllib.request.Request(BASE + path, method=method)
    req.add_header("Authorization", "Basic " + AUTH)
    req.add_header("Content-Type", "application/json")
    data = json.dumps(body).encode() if body is not None else None
    try:
        with urllib.request.urlopen(req, data=data, timeout=15) as r:
            return r.status, json.loads(r.read() or b"null")
    except urllib.error.HTTPError as e:
        return e.code, json.loads(e.read() or b"{}")


BOOLS = ["yes", "no", "true", "false"]

# (path, item, field, extra candidates). item None = settings singleton.
PROBES = []
for f in ["bytes", "dst-address", "dst-address-mask", "dst-mac-address",
          "dst-port", "first-forwarded", "gateway", "icmp-code", "icmp-type",
          "igmp-type", "in-interface", "ip-header-length", "ipv6-flow-label",
          "is-multicast", "last-forwarded", "nat-dst-address", "nat-dst-port",
          "nat-events", "nat-src-address", "nat-src-port", "out-interface",
          "packets", "protocol", "src-address", "src-address-mask",
          "src-mac-address", "src-port", "sys-init-time", "tcp-ack-num",
          "tcp-flags", "tcp-seq-num", "tcp-window-size", "tos", "ttl",
          "udp-length"]:
    PROBES.append(("/ip/traffic-flow/ipfix", None, f, []))
PROBES += [
    ("/ip/cloud", None, "ddns-enabled", ["auto"]),
    ("/ip/cloud", None, "update-time", ["auto"]),
    ("/ip/firewall/connection/tracking", None, "loose-tcp-tracking", ["auto"]),
    ("/ipv6/settings", None, "accept-redirects", ["yes-if-forwarding-disabled"]),
    ("/interface/6to4", "tp-6to4", "dont-fragment", ["inherit"]),
    ("/interface/gre", "tp-gre", "dont-fragment", ["inherit"]),
    ("/interface/eoip", "tp-eoip", "dont-fragment", ["inherit"]),
    ("/interface/ipip", "tp-ipip", "dont-fragment", ["inherit"]),
    ("/interface/bridge", "tp-br", "port-cost-mode", ["short", "long"]),
    ("/interface/ethernet", "ether3", "sfp-rate-select", ["high", "low"]),
]

FIXTURES = [
    ("/interface/6to4", {"name": "tp-6to4"}),
    ("/interface/gre", {"name": "tp-gre", "remote-address": "192.0.2.6"}),
    ("/interface/eoip", {"name": "tp-eoip", "tunnel-id": "77", "remote-address": "192.0.2.7"}),
    ("/interface/ipip", {"name": "tp-ipip", "remote-address": "192.0.2.8"}),
    ("/interface/bridge", {"name": "tp-br"}),
]


def main():
    code, sysres = rest("GET", "/system/resource")
    if code >= 300:
        sys.exit(f"router unreachable at {BASE} — start it with hack/chr/run.sh")
    version = sysres.get("version", "")

    created = []
    for path, body in FIXTURES:
        code, res = rest("PUT", path, body)
        if code < 300:
            created.append((path, body["name"]))
        else:
            print(f"FIXTURE FAIL {path}: {code} {res}", file=sys.stderr)

    results = []
    for path, item, field, extra in PROBES:
        tgt = path + ("/" + item if item else "")
        accepted = {}
        for v in BOOLS + extra + ["tpbogus"]:
            if item:
                code, _ = rest("PATCH", tgt, {field: v})
            else:
                code, _ = rest("POST", path + "/set", {field: v})
            if code < 300:
                code2, cur = rest("GET", tgt)
                if isinstance(cur, list):
                    cur = cur[0] if cur else {}
                accepted[v] = cur.get(field, "?") if code2 < 300 else "?"
        results.append({"path": path, "item": item, "field": field, "accepted": accepted})
        acc = " ".join(f"{v}->{s}" if v != s else v for v, s in accepted.items())
        print(f"{tgt:32} {field:28} {acc or 'NOTHING ACCEPTED'}", file=sys.stderr)

    for path, name in created:
        rest("DELETE", f"{path}/{name}")

    json.dump({
        "routeros_version": version,
        "probed_on": "CHR x86_64 under QEMU",
        "generator": "hack/schemaaudit/typeprobe.py",
        "note": "accepted maps candidate value -> value stored by the router; "
                "'?' means the field was not readable back (inconclusive on "
                "CHR virtual hardware); absent candidates were rejected",
        "results": results,
    }, sys.stdout, indent=2)
    print()


if __name__ == "__main__":
    main()
