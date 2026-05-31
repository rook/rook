---
title: AI Guidelines
---

Rook's AI guidelines are focused on preserving trust. Thousands of Rook users trust Rook and its maintainers to release enterprise-grade software with as few issues as possible and with as much transparency as possible. Each person who contributes code to Rook must be able to do so in a way that preserves maintainer trust, and by extension, user trust.

## Legal requirements

As a CNCF project, Rook follows the [Linux Foundation Guidelines](https://www.linuxfoundation.org/legal/generative-ai) for ensuring legal compliance. These guidelines require contributors to check (in summary):

1. AI tool restrictions: Contributors must ensure the AI tool's terms don't impose contractual limitations that conflict with the project's license.
2. Third-party content permissions: If generated output contains copyrighted materials from others, contributors must confirm proper permissions exist (via compatible license) and provide license information when contributing.

In order to ensure legal guidelines, Rook requires human involvement and engagement and does not allow fully-autonomous or near-fully-autonomous "bots" to submit issues or pull requests. If Rook maintainers have reasonable cause to believe a contribution does not have human oversight, it will be rejected.

## Transparency requirements

1. Contributors should disclose in pull request descriptions how AI tools assisted in development. E.g., generating substantial functions, documentation, and/or unit tests.
2. Rook requires human sign-off. Signing commits using an AI tool, or listing AI tooling in commits using `co-authored-by`, `assisted-by`, `generated-by`, or using similar commit trailers is not necessary.

## Best practices to respect maintainer time

AI models are making it much easier to contribute code to open source, and maintainer time is limited.

1. Consider opening an issue following [issue guidelines](development-flow.md#begin-with-an-issue) before contributing.
2. Do not submit large AI-generated PRs without previous design discussion. Follow [Kubernetes's guidelines for large/automatic edits](https://www.kubernetes.dev/docs/guide/pull-requests/#large-or-automatic-edits) as needed.
3. Do not use LLM output to write commit messages or GitHub comments. Reviewers are interested in knowing your reasoning about the code you submitted for review. It's yours to own and explain. Automated responses may result in a PR being rejected.
