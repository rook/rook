---
title: Debugging CI over SSH
---

Rook's integration and canary suites run against a live Kubernetes + Ceph
cluster on GitHub-hosted runners. When a test fails (or misbehaves) in CI but
cannot be reproduced locally, developers can open an interactive SSH session
into the runner and inspect the running cluster directly.

The session is provided by [upterm](https://github.com/owenthereal/upterm) via
the shared [`ci-ssh-debug`](https://github.com/rook/rook/blob/master/.github/workflows/ci-ssh-debug/action.yml)
composite action. GitHub-hosted runners are ephemeral and only allow outbound
connections, so the runner dials out to an upterm relay and you connect through
that relay — there is no inbound SSH to the runner itself.

## Prerequisites

- **Your SSH public key must be registered on GitHub** (Settings → SSH and GPG
  keys). Access is granted by fetching keys from `https://github.com/<user>.keys`.
- **You must be the user who triggered the run.** The session only accepts the
  triggering actor's SSH keys. Because opening a session already requires write
  access (applying the `debug-ci` label or re-running a job), the person who
  requests the session is the person who connects.

## Opening a session

A debug session is opened only when it is explicitly requested — it never runs
on ordinary CI. There are two ways to request one:

1. **Apply the `debug-ci` label to the pull request**, then push a commit or
   re-run the jobs. Applying a label requires write access to the repository, so
   the label doubles as the authorization gate. Every suite that runs will open
   a session at its pre-job and post-job debug steps.
2. **Re-run a job with debug logging enabled** ("Re-run jobs" → "Enable debug
   logging"). This is the quickest way to reopen a session on a job that has
   already finished.

Sessions are opened at two points in each suite:

- **Pre-job** — before the cluster is created, to inspect the environment.
- **Post-job** — after the test finishes (and logs are collected), to inspect
  the final state.

## Connecting

1. Open the running (or re-run) job in the GitHub Actions UI.
2. Find the connection command in one of two places:
    - The **Summary** tab of the run shows a "🔧 CI SSH debug session" panel with
      instructions. The Summary updates live while the job runs.
    - The **set up upterm session** step's log prints the `ssh …@…` /
      `upterm …` connection command.
3. Run the printed `ssh` command from a machine whose key is registered and
   allowlisted.

Once connected you land on the runner with the workspace at
`/home/runner/work/rook/rook`. `kubectl` is configured against the test cluster,
so you can inspect pods, logs, CephCluster status, and the Ceph toolbox as
usual.

The session **closes automatically after 15 minutes with no connection**, so a
requested-but-unused session never holds a runner (and CI minutes) open
indefinitely. Because upterm has no detached mode, a job that reaches a debug
step will pause there until you connect or the timeout elapses.

## Configuration

No configuration is required — access is always restricted to the run's
triggering actor. The `ci-ssh-debug` action exposes one optional input,
`upterm-server`, the address of a self-hosted
[`uptermd`](https://github.com/owenthereal/upterm) relay (e.g.
`uptermd.example.com:22`) for full independence from the public relay. It
defaults to the public `uptermd.upterm.dev:22` server.

## Security notes

- Access is gated by the **triggering actor's SSH keys**, so the connection
  command being visible in public logs does not grant a shell to the public.
- Sessions only open in the `rook` organization's repositories (or on a manual
  re-run); they are never opened automatically.
- The session runs with the job's `GITHUB_TOKEN`; for pull requests from forks
  that token is read-only and no repository secrets are available.

## Implementation

`ci-ssh-debug` is the single source of truth. The older `tmate_debug` (pre-job)
and `upterm_debug` (post-job) actions are thin compatibility shims that delegate
to it, so all existing suites use the same implementation. tmate is no longer
used — its public relay proved unreliable from GitHub-hosted runners.
