// mdv browser runtime: resolve theme, fetch the rendered fragment, swap it into
// the DOM, (re)build the TOC, run Mermaid, and live-reload over SSE.
// All behavior lives here because the CSP forbids inline scripts.
(function () {
  "use strict";

  var cfg = document.body.dataset;
  var docPath = cfg.path || "";
  var themePref = cfg.theme || "auto";

  var docEl = document.getElementById("doc");
  var tocEl = document.getElementById("toc");
  var statusEl = document.getElementById("status");

  var mermaidLoaded = false;
  var statusTimer = null;

  // ---- theme ----
  function resolveTheme() {
    if (themePref === "light" || themePref === "dark") return themePref;
    return window.matchMedia &&
      window.matchMedia("(prefers-color-scheme: dark)").matches
      ? "dark"
      : "light";
  }

  function applyTheme() {
    document.documentElement.setAttribute("data-theme", resolveTheme());
  }

  applyTheme();
  if (window.matchMedia && themePref === "auto") {
    window
      .matchMedia("(prefers-color-scheme: dark)")
      .addEventListener("change", function () {
        applyTheme();
        if (mermaidLoaded) renderMermaid();
      });
  }

  // The index page has no document to render; theme handling above is enough.
  if (!docEl || !docPath) {
    return;
  }

  // ---- status bar ----
  function showStatus(text) {
    if (!statusEl) return;
    statusEl.textContent = text;
    statusEl.classList.add("show");
    clearTimeout(statusTimer);
    statusTimer = setTimeout(function () {
      statusEl.classList.remove("show");
    }, 1200);
  }

  // ---- TOC ----
  function buildTOC(toc) {
    if (!tocEl) return;
    if (!toc || toc.length < 2) {
      tocEl.classList.add("hidden");
      tocEl.innerHTML = "";
      return;
    }
    tocEl.classList.remove("hidden");
    var ul = document.createElement("ul");
    toc.forEach(function (e) {
      var li = document.createElement("li");
      li.className = "lvl-" + e.level;
      var a = document.createElement("a");
      a.href = "#" + e.id;
      a.textContent = e.text;
      li.appendChild(a);
      ul.appendChild(li);
    });
    tocEl.innerHTML = "";
    tocEl.appendChild(ul);
  }

  // ---- Mermaid ----
  function renderMermaid() {
    if (!window.mermaid) return;
    try {
      window.mermaid.initialize({
        startOnLoad: false,
        securityLevel: "strict",
        theme: resolveTheme() === "dark" ? "dark" : "default",
      });
      window.mermaid.run({ querySelector: ".mermaid" });
    } catch (err) {
      console.error("mermaid render failed", err);
    }
  }

  function ensureMermaid(cb) {
    if (mermaidLoaded) {
      cb();
      return;
    }
    var s = document.createElement("script");
    s.src = "/__mdv/assets/mermaid.min.js";
    s.onload = function () {
      mermaidLoaded = true;
      cb();
    };
    s.onerror = function () {
      console.error("failed to load mermaid.min.js");
    };
    document.head.appendChild(s);
  }

  // ---- fragment load ----
  function load(preserveScroll) {
    var y = preserveScroll ? window.scrollY : 0;
    fetch("/__mdv/fragment?path=" + encodeURIComponent(docPath), {
      cache: "no-store",
    })
      .then(function (r) {
        if (!r.ok) throw new Error("fragment HTTP " + r.status);
        return r.json();
      })
      .then(function (data) {
        docEl.innerHTML = data.html;
        if (data.title) document.title = data.title;
        buildTOC(data.toc);
        if (data.hasMermaid) {
          ensureMermaid(renderMermaid);
        }
        window.scrollTo(0, y);
      })
      .catch(function (err) {
        console.error(err);
        showStatus("load error");
      });
  }

  // ---- live reload (SSE) ----
  function connect() {
    var es = new EventSource(
      "/__mdv/events?path=" + encodeURIComponent(docPath)
    );
    es.onmessage = function (ev) {
      if (ev.data === "reload") {
        load(true);
        showStatus("reloaded");
      }
    };
    es.onerror = function () {
      es.close();
      setTimeout(connect, 1500);
    };
  }

  load(false);
  connect();
})();
