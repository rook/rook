const admonition_identifier = "!!!"
const body_tab_space_indent = 4

module.exports = {
  "names": [ "mkdocs-admonitions" ],
  "description": "Enforce mkdocs admonitions are formatted properly",
  "tags": [ "test" ],
  "parser": "none",
  "function": function rule(params, onError) {
    const { lines } = params;
    const countTabSpaces = (line) => {
      start_spaces = /^\s*/;
      return line.match(start_spaces)[0].length
    };
    lines.forEach((line, i) => {
      const thisLineNumber = i+1 // "lineNumber" field is ones-based
      const bodyLineIndex = i+1 // body line should start after header
      const bodyLineNumber = bodyLineIndex + 1

      const isAdmonitionHeader = ( line.trimLeft().startsWith(admonition_identifier) )
      if ( !isAdmonitionHeader ) return;

      const admonition_start = line.indexOf(admonition_identifier)
      const expected_body_tab_spaces = ''.padStart(admonition_start+body_tab_space_indent, ' ')
      if ( lines[i+1].trim().length == 0 ) {
        // body should immediately follow header
        onError({
          "lineNumber": thisLineNumber + 1,
          "detail": "found blank line after admonition header -- body text should immediately follow header",
          "context": line,
          "fixInfo": {
            "lineNumber": thisLineNumber + 1,
            "deleteCount": -1, // delete the line
          },
        });
      } else if ( lines[bodyLineIndex] != expected_body_tab_spaces + lines[bodyLineIndex].trimStart() ) {
        // body should be indented exactly 4 spaces from start of header
        got_tab_spaces = countTabSpaces(lines[bodyLineIndex])
        onError({
          "lineNumber": bodyLineNumber,
          "detail": "admonition/callout body is not indented properly",
          "context": lines[bodyLineIndex],
          "fixInfo": {
            "lineNumber": bodyLineNumber,
            "deleteCount": got_tab_spaces,
            "insertText": expected_body_tab_spaces, // insert at beginning of lineNumber
          },
        });
      }
    });
  }
};
