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
  var toggleEl = document.getElementById("theme-toggle");
  var shellEl = document.getElementById("shell");
  var tocToggleEl = document.getElementById("toc-toggle");

  var sourceEl = document.getElementById("source");
  var sourceToggleEl = document.getElementById("source-toggle");
  var sourcePanelEl = document.getElementById("source-panel");
  var copyEl = document.getElementById("source-copy");
  var showingSource = false;
  var viewScroll = { preview: 0, source: 0 };

  var mermaidLoaded = false;
  var statusTimer = null;

  // ---- theme ----
  // Precedence: an explicit in-browser choice (localStorage) wins over the
  // server default (MDV_THEME / --theme); "auto" falls back to the OS setting.
  var THEME_KEY = "mdv-theme";

  function storedTheme() {
    try {
      var v = localStorage.getItem(THEME_KEY);
      return v === "light" || v === "dark" ? v : null;
    } catch (e) {
      return null;
    }
  }

  function storeTheme(t) {
    try {
      localStorage.setItem(THEME_KEY, t);
    } catch (e) {
      /* private mode / disabled storage: fall back to session-only */
    }
  }

  function systemTheme() {
    return window.matchMedia &&
      window.matchMedia("(prefers-color-scheme: dark)").matches
      ? "dark"
      : "light";
  }

  function resolveTheme() {
    var stored = storedTheme();
    if (stored) return stored;
    if (themePref === "light" || themePref === "dark") return themePref;
    return systemTheme();
  }

  function updateToggle() {
    if (!toggleEl) return;
    var next = resolveTheme() === "dark" ? "light" : "dark";
    // Show the icon of the mode the button switches to.
    toggleEl.textContent = next === "dark" ? "🌙" : "☀️";
    toggleEl.setAttribute(
      "aria-label",
      next === "dark" ? "Switch to dark theme" : "Switch to light theme"
    );
    toggleEl.title = toggleEl.getAttribute("aria-label");
  }

  function applyTheme() {
    document.documentElement.setAttribute("data-theme", resolveTheme());
    updateToggle();
  }

  applyTheme();

  if (toggleEl) {
    toggleEl.addEventListener("click", function () {
      storeTheme(resolveTheme() === "dark" ? "light" : "dark");
      applyTheme();
      if (mermaidLoaded) renderMermaid();
    });
  }

  // Follow the OS only while no explicit choice is stored.
  if (window.matchMedia) {
    window
      .matchMedia("(prefers-color-scheme: dark)")
      .addEventListener("change", function () {
        if (storedTheme()) return;
        applyTheme();
        if (mermaidLoaded) renderMermaid();
      });
  }

  // The index page has no document to render; theme handling above is enough.
  if (!docEl || !docPath) {
    return;
  }

  // Keep both views mounted so switching preserves diagrams and selections.
  if (sourceToggleEl && sourceEl && sourcePanelEl) {
    sourceToggleEl.addEventListener("click", function () {
      viewScroll[showingSource ? "source" : "preview"] = window.scrollY;
      showingSource = !showingSource;
      docEl.hidden = showingSource;
      sourcePanelEl.hidden = !showingSource;
      if (shellEl) shellEl.classList.toggle("source-view", showingSource);
      if (tocToggleEl) tocToggleEl.hidden = showingSource;
      sourceToggleEl.title = showingSource ? "Show preview" : "Show Markdown source";
      sourceToggleEl.setAttribute("aria-label", sourceToggleEl.title);
      sourceToggleEl.setAttribute("aria-pressed", String(showingSource));
      window.scrollTo(0, viewScroll[showingSource ? "source" : "preview"]);
    });
  }

  function bindCopy(button, getText, label) {
    var timer = null;
    button.classList.remove("copied", "copy-error");
    button.setAttribute("aria-label", label);
    button.title = label;
    function feedback(success, message) {
      clearTimeout(timer);
      button.classList.remove("copied", "copy-error");
      button.classList.add(success ? "copied" : "copy-error");
      button.setAttribute("aria-label", message);
      button.title = message;
      showStatus(message, success ? "success" : "error");
      timer = setTimeout(function () {
        button.classList.remove("copied", "copy-error");
        button.setAttribute("aria-label", label);
        button.title = label;
      }, success ? 1800 : 4000);
    }
    button.addEventListener("click", function () {
      if (!navigator.clipboard || !navigator.clipboard.writeText) {
        feedback(false, "Copy unavailable. Select the text to copy.");
        return;
      }
      navigator.clipboard.writeText(getText()).then(function () {
        feedback(true, "Copied!");
      }).catch(function () {
        feedback(false, "Copy failed. Select the text to copy.");
      });
    });
  }

  if (copyEl && sourceEl) {
    bindCopy(copyEl, function () { return sourceEl.textContent; }, "Copy Markdown");
  }

  function addCodeCopyButtons() {
    if (!copyEl) return;
    docEl.querySelectorAll("pre").forEach(function (pre) {
      var code = pre.querySelector("code");
      if (!code) return;
      var panel = document.createElement("div");
      panel.className = "mdv-code-panel";
      var button = copyEl.cloneNode(true);
      button.removeAttribute("id");
      button.classList.remove("copied");
      bindCopy(button, function () { return code.textContent; }, "Copy code");
      pre.parentNode.insertBefore(panel, pre);
      panel.appendChild(button);
      panel.appendChild(pre);
    });
  }

  // ---- status bar ----
  function showStatus(text, kind) {
    if (!statusEl) return;
    statusEl.textContent = text;
    statusEl.setAttribute("data-kind", kind || "info");
    statusEl.classList.add("show");
    clearTimeout(statusTimer);
    statusTimer = setTimeout(function () {
      statusEl.classList.remove("show");
    }, kind === "error" ? 4000 : 1800);
  }

  // ---- sidebar (TOC) collapse ----
  // The collapsed choice is remembered per-browser; the toggle button only
  // appears when the current document actually has a TOC to hide.
  var TOC_COLLAPSE_KEY = "mdv-toc-collapsed";

  function storedCollapsed() {
    try {
      return localStorage.getItem(TOC_COLLAPSE_KEY) === "1";
    } catch (e) {
      return false;
    }
  }

  function storeCollapsed(collapsed) {
    try {
      localStorage.setItem(TOC_COLLAPSE_KEY, collapsed ? "1" : "0");
    } catch (e) {
      /* private mode / disabled storage: fall back to session-only */
    }
  }

  function applyTocCollapsed(collapsed) {
    if (shellEl) shellEl.classList.toggle("toc-collapsed", collapsed);
    if (!tocToggleEl) return;
    tocToggleEl.textContent = collapsed ? "☰" : "«";
    tocToggleEl.setAttribute("aria-expanded", collapsed ? "false" : "true");
    var label = collapsed ? "Show sidebar" : "Hide sidebar";
    tocToggleEl.setAttribute("aria-label", label);
    tocToggleEl.title = label;
  }

  if (tocToggleEl) {
    tocToggleEl.addEventListener("click", function () {
      var next = !(shellEl && shellEl.classList.contains("toc-collapsed"));
      storeCollapsed(next);
      applyTocCollapsed(next);
    });
  }

  // ---- TOC ----
  function buildTOC(toc) {
    if (!tocEl) return;
    if (!toc || toc.length < 2) {
      tocEl.classList.add("hidden");
      tocEl.innerHTML = "";
      if (tocToggleEl) tocToggleEl.classList.add("hidden");
      return;
    }
    tocEl.classList.remove("hidden");
    if (tocToggleEl) tocToggleEl.classList.remove("hidden");
    applyTocCollapsed(storedCollapsed());
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
        addCodeCopyButtons();
        // Raw Markdown must stay text, even when it contains HTML or scripts.
        if (sourceEl) sourceEl.textContent = data.source;
        if (sourceToggleEl) sourceToggleEl.disabled = false;
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
