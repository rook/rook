module.exports = {
  "config": {
    "default": false, // no default rules enabled
    "extends": null, // no default rules enabled
    "list-indent": true, // all list items must be indented at the same level
    "ul-indent": {
      "indent": 4, // 4 spaces for tabs
    },
    "no-hard-tabs": {
      "spaces_per_tab": 4, // 4 spaces for tabs
    },
    "ol-prefix": {
      // require fully-numbered lists. this rule helps ensure that code blocks in between ordered
      // list items (which require surrounding spaces) don't break lists
      "style": "ordered",
    },
    "blanks-around-lists": true, // blank lines around lists are required
    "blanks-around-fences": { // blank lines around code blocks (fences) are required
      "list_items": true, /// ... including in lists
    },
    "code-block-style": {
      // do not allow implicit code blocks. ensure code blocks have fences so we know explicitly
      // what will be rendered
      "style": "fenced",
    },
    "fenced-code-language": {
      // enforce code blocks must have language specified
      // this helps ensure rendering is as intended, and it helps doc-wide searches for code blocks
      language_only: true,
    },
    "no-duplicate-heading": true, // do not allow duplicate headings
    "link-fragments": true, // validate links to headings within a doc
    "single-trailing-newline": true, // require single trailing newline in docs
    "no-multiple-blanks": {
      // allow max 2 blank lines in markdown docs
      "maximum": 2,
    },

    // custom rules for Rook!
    "alerts": true, // checking blockquote alert types and rejecting legacy admonition syntax
    "strict-tab-spacing": true, // enforce strict 4-space tabs

  },
 "customRules": [
    "./tests/scripts/markdownlint-alerts.js",
    "./tests/scripts/markdownlint-tab-spacing.js",
  ],
};
