#!/usr/bin/env python3

import sys

import ruamel.yaml

def log(*values):
    print(*values, file=sys.stderr, flush=True)

rbac_kinds = [
    "PodSecurityPolicy",
    "ServiceAccount",
    "ClusterRole",
    "ClusterRoleBinding",
    "Role",
    "RoleBinding",
]

yaml=ruamel.yaml.YAML()
# output lists in the form that is indented from the parent like below
# parent:
#   - list
#   - items
yaml.indent(sequence=4, offset=2)

in_docs = yaml.load_all(sys.stdin.read())

kept_docs = []

num_parsed = 0
num_kept = 0
for doc in in_docs:
    num_parsed += 1
    kind = doc["kind"]
    if kind not in rbac_kinds:
        # we don't want non-RBAC resources
        log("discarding doc with kind:", kind)
        continue

    log("keeping doc with kind:", kind)

    # helm adds '# Source: <file>' comments to the top of each yaml doc. Strip these.
    if doc.ca is not None and doc.ca.comment is not None:
        comments = doc.ca.comment[1]
        for comment in comments:
            if comment.value.startswith("# Source: ") and comment.value.endswith(".yaml\n"):
                log("dropping comment:", comment.value.strip())
                comments.remove(comment)

    kept_docs.append(doc)

log("docs processed:", num_parsed)
log("docs kept:", len(kept_docs))

# log(kept_docs)
yaml.dump_all(kept_docs, sys.stdout)
