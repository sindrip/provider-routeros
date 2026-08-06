#!/usr/bin/env python3
"""Schema audit: router truth vs upstream terraform-provider-routeros.

Diffs three sources and reports coverage and field gaps:
  - config/console-tree.json     authoritative structure from a live CHR
                                 (writable args = add/set arguments)
  - the manual's CLI reference   types, mandatory flags, read-only tables
                                 (downloaded into .work/schemaaudit/cliref)
  - upstream resource schemas    dumped via ./upstreamdump

Writes .work/schemaaudit/audit-report.json and prints a summary. The type
mismatch bucket reports where the manual and upstream disagree; those are
claims, not verdicts — settle them against a live CHR with typeprobe.py
(pinned results: config/type-verdicts.json).

Caveats learned the hard way:
  - Dotted args (aaa.called-format) are nested groups upstream models as one
    map field; not missing fields.
  - The manual nests sub-menus under deeper headings (###, ####).
  - Manual types describe the console's internal type, not the accepted REST
    surface: bool-backed enums (dont-fragment) reject yes/no but accept
    true/false plus their display labels. See config/type-verdicts.json.
"""
import json
import os
import re
import subprocess
import sys
import urllib.request
from collections import defaultdict
from concurrent.futures import ThreadPoolExecutor

HERE = os.path.dirname(os.path.abspath(__file__))
ROOT = os.path.dirname(os.path.dirname(HERE))
WORK = os.path.join(ROOT, ".work", "schemaaudit")
CLIREF = os.path.join(WORK, "cliref")
MANUAL_INDEX = "https://manual.mikrotik.com/llms.txt"

# Fields the router accepts everywhere or that are TF-side artifacts; not
# interesting as per-resource gaps.
IGNORE = {
    "comment", "disabled", "copy-from", "place-before", "dynamic", "invalid",
    "default", "numbers", "next-pool",
}


def fetch(url):
    with urllib.request.urlopen(url, timeout=30) as r:
        return r.read().decode()


def download_cliref():
    os.makedirs(CLIREF, exist_ok=True)
    urls = sorted(set(re.findall(
        r"https://manual\.mikrotik\.com/docs/cli-reference/[^)\s]*\.md",
        fetch(MANUAL_INDEX))))

    def get(url):
        name = url.split("/docs/cli-reference/")[1].replace("/", "__")
        path = os.path.join(CLIREF, name)
        if not os.path.exists(path):
            open(path, "w").write(fetch(url))
    with ThreadPoolExecutor(8) as pool:
        list(pool.map(get, urls))
    return len(urls)


def dump_upstream():
    # upstreamdump is its own module; go must run from inside it, not from
    # the parent (which the root go.mod claims).
    out = subprocess.run(["go", "run", "."],
                         cwd=os.path.join(HERE, "upstreamdump"),
                         capture_output=True, text=True)
    if out.returncode != 0:
        sys.exit(f"upstreamdump failed:\n{out.stderr}")
    return json.loads(out.stdout)


def parse_manual():
    row = re.compile(r'<ArgTableRow arg="([^"]+)"(?:\s+typ="([^"]*)")?([^>]*)>', re.S)
    section = re.compile(r"^#{2,6} (\S+)$", re.M)
    table = re.compile(r'<ArgTable c1="([^"]+)"[^>]*>(.*?)</ArgTable>', re.S)
    manual = {}
    for fn in os.listdir(CLIREF):
        text = open(os.path.join(CLIREF, fn)).read()
        sections = list(section.finditer(text))
        for i, m in enumerate(sections):
            body = text[m.end():sections[i + 1].start() if i + 1 < len(sections) else len(text)]
            entry = manual.setdefault(m.group(1), {"writable": {}, "readonly": {}})
            for tm in table.finditer(body):
                kind, rows = tm.group(1), tm.group(2)
                for rm in row.finditer(rows):
                    arg, typ, rest = rm.group(1), rm.group(2) or "", rm.group(3)
                    if kind == "Argument":
                        entry["writable"][arg] = (typ, 'mandatory="1"' in rest)
                    elif kind == "Read-only Argument":
                        entry["readonly"][arg] = typ
    return manual


def parse_console():
    tree = json.load(open(os.path.join(ROOT, "config", "console-tree.json")))
    console = {}

    def walk(nodes, path):
        for n in nodes:
            p = path + [n["name"]]
            if n["node_type"] in ("dir", "path"):
                kids = n.get("children") or []
                cmds = {k["name"]: k for k in kids if k["node_type"] == "cmd"}
                args = set()
                for cmd in ("add", "set"):
                    for a in (cmds.get(cmd, {}).get("children") or []):
                        if a["node_type"] == "arg":
                            args.add(a["name"])
                args -= {"numbers"}  # 'set' addressing arg, not a property
                console["/".join(p)] = {
                    "args": args, "add": "add" in cmds, "set": "set" in cmds,
                }
                walk(kids, p)
    walk(tree["tree"], [])
    return console


def main():
    pages = download_cliref()
    manual = parse_manual()
    console = parse_console()
    upstream = dump_upstream()

    by_path = defaultdict(list)
    for r in upstream:
        if r["path"]:
            by_path[r["path"].strip("/")].append(r)
    kebab = lambda s: s.replace("_", "-")

    report = {"missing_resources": [], "missing_fields": {}, "unknown_fields": {},
              "type_mismatches": [], "missing_readonly": {}}

    for path, info in sorted(console.items()):
        if not (info["add"] or info["set"]) or path in by_path or not info["args"]:
            continue
        report["missing_resources"].append({
            "path": path,
            "kind": "collection" if info["add"] else "settings",
            "fields": len(info["args"]),
        })

    for path, resources in sorted(by_path.items()):
        info = console.get(path)
        if info is None:
            continue
        man = manual.get(path, {"writable": {}, "readonly": {}})
        router_args = info["args"]
        for r in resources:
            up = {kebab(f) for f in r["fields"]}
            group_ok = lambda a: "." in a and a.split(".", 1)[0] in up
            missing = sorted(a for a in router_args - up - IGNORE if not group_ok(a))
            if missing:
                report["missing_fields"][r["resource"]] = missing
            unknown = sorted(up - (router_args | set(man["readonly"]) | IGNORE))
            if unknown:
                report["unknown_fields"][r["resource"]] = unknown
            ro_missing = sorted(set(man["readonly"]) - up - IGNORE)
            if ro_missing:
                report["missing_readonly"][r["resource"]] = ro_missing
            for f, spec in r["fields"].items():
                kf = kebab(f)
                man_typ = man["writable"].get(kf, (None,))[0]
                if not man_typ:
                    continue
                if man_typ == "bool" and spec["type"] != "TypeBool":
                    report["type_mismatches"].append(
                        {"resource": r["resource"], "field": kf,
                         "manual": "bool", "tf": spec["type"]})
                if man_typ.startswith("num") and spec["type"] == "TypeBool":
                    report["type_mismatches"].append(
                        {"resource": r["resource"], "field": kf,
                         "manual": man_typ, "tf": spec["type"]})

    out = os.path.join(WORK, "audit-report.json")
    json.dump(report, open(out, "w"), indent=2)

    covered = sum(1 for p in by_path if p in console)
    print(f"manual pages: {pages} ({len(manual)} menu sections)  console menus: {len(console)}")
    print(f"upstream paths: {len(by_path)} ({covered} matched to console tree)")
    print(f"router menus with no upstream resource: {len(report['missing_resources'])}")
    print(f"resources missing writable fields: {len(report['missing_fields'])} "
          f"({sum(len(v) for v in report['missing_fields'].values())} fields)")
    print(f"resources with unknown (router-absent) fields: {len(report['unknown_fields'])} "
          f"({sum(len(v) for v in report['unknown_fields'].values())} fields)")
    print(f"resources missing read-only fields: {len(report['missing_readonly'])} "
          f"({sum(len(v) for v in report['missing_readonly'].values())} fields)")
    print(f"type mismatches: {len(report['type_mismatches'])}")
    print(f"report: {out}")


if __name__ == "__main__":
    main()
