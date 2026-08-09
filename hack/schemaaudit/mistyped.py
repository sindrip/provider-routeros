#!/usr/bin/env python3
"""Find upstream schema fields whose Terraform type cannot hold the router's.

Diffs two sources:
  - config/arg-types.json   the router's own answer for every writable
                            console argument, from hack/typedump
  - upstream resource schemas, dumped via ./upstreamdump

Writes .work/schemaaudit/mistyped-report.json and prints a summary. Unlike
audit.py's type_mismatches bucket — which reports where the *manual* and
upstream disagree, and is a claim — the router is the authority here, so the
"cannot" buckets are verdicts.

The one thing the console does not settle is the REST vocabulary. A console
enum of yes/no is routinely accepted as true/false over REST (see
config/type-verdicts.json, and audit.py's note on dont-fragment), so a
two-value boolean-ish vocabulary says nothing against a TypeBool. What a
TypeBool provably cannot hold is a *third* state: that is the advertise-dns
case config/mistyped_enums.go was written for, and it is the bucket to read
first.

Usage:
    python3 hack/schemaaudit/mistyped.py
"""
import json
import os
import sys
from collections import defaultdict

from audit import IGNORE, dump_upstream

HERE = os.path.dirname(os.path.abspath(__file__))
ROOT = os.path.dirname(os.path.dirname(HERE))
WORK = os.path.join(ROOT, ".work", "schemaaudit")

# Values a TypeBool round-trips regardless of which spelling the console
# reports; a vocabulary inside this set is no evidence of a mistype.
BOOLISH = {"yes", "no", "true", "false"}

# Scalar type labels (from the syntax grammar) that a bool cannot carry.
NON_BOOL_SCALARS = {
    "integer number", "time interval", "IP address", "IPv6 address",
    "MAC address", "IP prefix",
}

# Vocabulary size past which an enum is a namespace rather than a closed set
# worth pinning into CRD validation (log topics, firewall address lists).
ENUM_VALIDATION_MAX = 24

# "!" completes the console's negation prefix on firewall matchers; it is an
# operator, not a value, and says nothing about the argument's type.
NEGATION = "!"


def vocabulary(a):
    """The argument's values with console operators removed."""
    return [v for v in (a.get("values") or []) if v != NEGATION]


def load_arg_types():
    path = os.path.join(ROOT, "config", "arg-types.json")
    if not os.path.exists(path):
        sys.exit(f"{path} not found; run hack/typedump first")
    d = json.load(open(path))
    return d, {(a["path"], a["arg"]): a for a in d["args"]}


def main():
    types_doc, by_key = load_arg_types()
    upstream = dump_upstream()

    report = defaultdict(list)
    kebab = lambda s: s.replace("_", "-")
    matched = unmatched = 0

    for r in sorted(upstream, key=lambda r: r["resource"]):
        path = (r["path"] or "").strip("/")
        if not path:
            continue
        for fname, spec in sorted(r["fields"].items()):
            arg = kebab(fname)
            if arg in IGNORE:
                continue
            a = by_key.get((path, arg))
            if a is None:
                unmatched += 1
                continue
            matched += 1

            tf, kind, values = spec["type"], a["kind"], vocabulary(a)
            entry = {
                "resource": r["resource"], "field": fname, "path": path,
                "tf_type": tf, "router_kind": kind,
                # Writable mistypes corrupt what is sent; read-only ones only
                # corrupt what is observed into atProvider. Both are wrong,
                # but only the first can write to the device.
                "access": a.get("access", "writable"),
            }
            if values:
                entry["router_values"] = values
            if a.get("types"):
                entry["router_types"] = a["types"]
            if a.get("ranges"):
                entry["router_ranges"] = a["ranges"]
            if fname in (r.get("skip_fields") or []):
                entry["skipped_by_serializer"] = True

            if tf == "TypeBool":
                extra = [v for v in values if v not in BOOLISH]
                if kind in ("enum", "open-enum") and extra and len(values) >= 3:
                    # Three states will not fit in two. Definite.
                    entry["why"] = (
                        f"router accepts {len(values)} values "
                        f"({', '.join(values)}); a bool drops {', '.join(extra)}")
                    report["bool_too_small"].append(entry)
                elif kind in ("enum", "open-enum") and extra:
                    # dont-fragment class: a two-value non-yes/no vocabulary.
                    # Semantically boolean, but confirm the REST round-trip
                    # before trusting it (typeprobe.py).
                    entry["why"] = f"bool-backed vocabulary {', '.join(values)}; verify round-trip"
                    report["bool_backed_enum"].append(entry)
                elif kind == "scalar":
                    hit = sorted(set(a.get("types") or []) & NON_BOOL_SCALARS)
                    if hit:
                        entry["why"] = f"router wants {', '.join(hit)}, not a boolean"
                        report["bool_not_boolean"].append(entry)
            elif tf in ("TypeInt", "TypeFloat"):
                if kind in ("enum", "open-enum") and not all(v.isdigit() for v in values):
                    entry["why"] = f"router accepts non-numeric {', '.join(values)}"
                    report["numeric_not_numeric"].append(entry)
            elif tf == "TypeString":
                if kind == "enum" and 1 < len(values) <= ENUM_VALIDATION_MAX:
                    # Not a bug: a closed vocabulary the CRD could validate.
                    entry["why"] = "closed vocabulary available for CRD enum validation"
                    report["enum_candidates"].append(entry)

    os.makedirs(WORK, exist_ok=True)
    out = os.path.join(WORK, "mistyped-report.json")
    payload = {
        "routeros_version": types_doc.get("routeros_version"),
        "matched_fields": matched, "unmatched_fields": unmatched,
        **{k: v for k, v in sorted(report.items())},
    }
    json.dump(payload, open(out, "w"), indent=2)

    print(f"RouterOS {types_doc.get('routeros_version')}  "
          f"fields matched to a router argument: {matched} ({unmatched} unmatched)")
    print()
    print(f"  bool_too_small      {len(report['bool_too_small']):4d}  "
          "bool cannot hold the router's third state  <-- verdicts")
    print(f"  bool_not_boolean    {len(report['bool_not_boolean']):4d}  "
          "bool where the router wants a number/time/address")
    print(f"  numeric_not_numeric {len(report['numeric_not_numeric']):4d}  "
          "int/float where the router accepts words")
    print(f"  bool_backed_enum    {len(report['bool_backed_enum']):4d}  "
          "two-value non-yes/no vocabulary; verify round-trip")
    print(f"  enum_candidates     {len(report['enum_candidates']):4d}  "
          "string fields with a closed vocabulary (not bugs)")
    print()
    for e in report["bool_too_small"]:
        print(f"  {e['resource']}.{e['field']}: {e['why']}")
    for e in report["bool_not_boolean"]:
        print(f"  {e['resource']}.{e['field']}: {e['why']}")
    print(f"\nreport: {out}")


if __name__ == "__main__":
    main()
