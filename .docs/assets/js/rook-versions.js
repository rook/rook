// Version selector and outdated version banner.
//
// The published docs live in <root>/<version>/ next to <root>/versions.json, which lists
// [{"version": "v1.21", "title": "v1.21", "aliases": ["latest-release"]}, ...]. The same file is
// read by the version selector of the older docs built with MkDocs. Without versions.json, as in a
// local preview, nothing is shown.
document.addEventListener("DOMContentLoaded", async function () {
  const siteRoot = new URL(document.querySelector('meta[name="rook-site-root"]').content, location.href);
  const root = new URL("../", siteRoot);

  let versions;
  try {
    const resp = await fetch(new URL("versions.json", root));
    if (!resp.ok) return;
    versions = await resp.json();
  } catch (e) {
    return;
  }
  if (!location.pathname.startsWith(root.pathname)) return;

  // the version, or alias, in the URL, and the page path below it
  const [name, ...rest] = location.pathname.slice(root.pathname.length).split("/");
  const pagePath = rest.join("/");
  const current = versions.find((v) => v.version === name || (v.aliases || []).includes(name));
  if (!current) return;

  const latest = versions.find((v) => (v.aliases || []).includes("latest-release"));
  if (latest && latest !== current) {
    const banner = document.querySelector("[data-rook-outdated]");
    banner.querySelector("a").href = new URL("latest-release/", root).href;
    banner.hidden = false;
  }

  const selector = document.querySelector("[data-rook-version]");
  const button = selector.querySelector("button");
  const menu = selector.querySelector("ul");
  button.textContent = current.title;
  for (const v of versions) {
    const li = document.createElement("li");
    const a = document.createElement("a");
    a.textContent = v.title;
    a.href = new URL(v.version + "/", root).href;
    a.setAttribute("role", "menuitem");
    if (v === current) a.setAttribute("aria-current", "true");
    // go to the same page in the other version when it exists there
    a.addEventListener("click", async function (ev) {
      ev.preventDefault();
      const samePage = new URL(v.version + "/" + pagePath, root).href;
      let target = a.href;
      try {
        const resp = await fetch(samePage, { method: "HEAD" });
        if (resp.ok) target = samePage + location.hash;
      } catch (e) {}
      location.href = target;
    });
    li.appendChild(a);
    menu.appendChild(li);
  }
  button.addEventListener("click", function () {
    menu.hidden = !menu.hidden;
    button.setAttribute("aria-expanded", String(!menu.hidden));
  });
  document.addEventListener("click", function (ev) {
    if (!selector.contains(ev.target)) {
      menu.hidden = true;
      button.setAttribute("aria-expanded", "false");
    }
  });
  selector.hidden = false;
});
