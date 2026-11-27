const supported_alert_types = [
  "NOTE", "TIP", "IMPORTANT", "WARNING", "CAUTION", // GitHub alert types
  "INFO", "EXAMPLE", "ADVANCED", "HINT", "WARN", "ATTENTION", "BUG", // former mkdocs admonition types
]
const legacy_admonition = /^\s*(!!!|\?\?\?)/
const alert_marker = /^\s*>\s*\[!([^\]]*)\](?:[+-])?(?:\s.*)?$/
const fence = /^\s*(```|~~~)/

module.exports = {
  "names": [ "alerts" ],
  "description": "Enforce blockquote alerts use a supported type and legacy mkdocs admonitions are not used",
  "tags": [ "test" ],
  "parser": "none",
  "function": function rule(params, onError) {
    const { lines } = params;
    let inFence = false
    lines.forEach((line, i) => {
      const lineNumber = i+1 // "lineNumber" field is ones-based
      if ( fence.test(line) ) {
        inFence = !inFence
        return;
      }
      if ( inFence ) return;

      if ( legacy_admonition.test(line) ) {
        onError({
          "lineNumber": lineNumber,
          "detail": "mkdocs admonition syntax ('!!!' or '???') is no longer rendered -- use a blockquote alert: '> [!NOTE]'",
          "context": line,
        });
        return;
      }

      const match = line.match(alert_marker)
      if ( match && !supported_alert_types.includes(match[1].toUpperCase()) ) {
        onError({
          "lineNumber": lineNumber,
          "detail": `unsupported alert type '${match[1]}' -- supported types: ${supported_alert_types.join(", ")}`,
          "context": line,
        });
      }
    });
  }
};
