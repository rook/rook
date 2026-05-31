// enforce that lines must be indented to a strict number of tab-spaces
// this lint rule is largely non-contextual and requires nearly all lines to adhere to the rule
// this is even true for ordered/unordered lists in which mkdocs **usually** allows spacing to match
// that of the parent line. but this rule is quick and dirty, and it does not harm to enforce
// indenting to 4-space boundaries to be entirely certain that mkdocs will render as intended.

const expected_tab_spaces = 4

// it's hard to know exactly what spacing the user intended, but we can make an educated guess
// considering 2 things:
//   1. most code editors are likely to indent to 2 spaces by default. therefore, suggest
//      that anything indented 2 spaces or more should be indented all the way
//   2. some users put a single space in front of bullets/numbers when starting a list.
//      therefore, suggest that anything indented only a single space should be non-indented
const suggested_spacing_indent_cutoff = 2

module.exports = {
    "names": [ "strict-tab-spacing" ],
    "description": "Enforce strict tab spacing",
    "tags": [ "test" ],
    "parser": "none",
    "function": function rule(params, onError) {
      const { lines } = params;
      const countTabSpaces = (line) => {
        start_spaces = /^\s*/;
        return line.match(start_spaces)[0].length
      };
      const generateTabSpaces = (count) => {
          return ''.padStart(count, ' ')
      }
      const isCodeBlockEnd = (line) => {
        trimmed = line.trim()
        return ( trimmed === "```" )
      }
      const isCodeBlockStart = (line) => {
        if ( isCodeBlockEnd(line) ) return false;
        trimmed = line.trim()
        return ( trimmed.startsWith("```") )
      }
      let code_block_depth = 0
      lines.forEach((line, i) => {
        // don't strictly enforce tab spacing inside code blocks
        if ( isCodeBlockStart(line) ) {
          code_block_depth++
          // code block start lines must be indented to a tab-space boundary
        } else if ( isCodeBlockEnd(line) ) {
          code_block_depth--
          // code block end lines must also be indented to a tab-space boundary
        } else if ( code_block_depth > 0 ) {
          // if inside of a code block, but not a start/end line, don't warn about spacing
          return
        }

        const thisLineNumber = i+1 // "lineNumber" field is ones-based
        got_tab_spaces = countTabSpaces(line)
        const floor = got_tab_spaces / expected_tab_spaces
        const remainder = got_tab_spaces % expected_tab_spaces
        if ( remainder != 0 ) {
          // begin with the assumption that the line should be non-indented
          suggested_spacing = generateTabSpaces(floor)
          if ( remainder >= suggested_spacing_indent_cutoff ) {
              suggested_spacing = generateTabSpaces(floor + expected_tab_spaces)
          }
          onError({
            "lineNumber": thisLineNumber,
            "detail": "lines must be indented to a strict boundary of " + expected_tab_spaces + " spaces",
            "context": line,
            "fixInfo": {
              "lineNumber": thisLineNumber,
              "deleteCount": got_tab_spaces,
              "insertText": suggested_spacing, // insert at beginning of lineNumber
            },
          });
        }
      });
    }
  };
