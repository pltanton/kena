(function () {
  var strings = document.body.dataset;
  var flash = document.getElementById("flash");
  var toast = document.getElementById("toast");
  var hideFlash = null;
  var hideToast = null;

  function show(msg) {
    flash.textContent = msg;
    flash.hidden = false;
    clearTimeout(hideFlash);
    hideFlash = setTimeout(function () { flash.hidden = true; }, 6000);
  }
  document.body.addEventListener("htmx:responseError", function () { show(strings.msgServer); });
  document.body.addEventListener("htmx:sendError", function (e) {
    if (!e.defaultPrevented) show(strings.msgNetwork);
  });
  flash.addEventListener("click", function () { flash.hidden = true; });

  function say(msg) {
    toast.hidden = false;
    toast.textContent = msg;
    toast.classList.remove("out");
    clearTimeout(hideToast);
    hideToast = setTimeout(function () {
      toast.classList.add("out");
      hideToast = setTimeout(function () { toast.hidden = true; }, 250);
    }, 1600);
  }

  window.kenaSay = say;

  function post(url, data) {
    return fetch(url, {method: "POST", headers: {"HX-Request": "true"}, body: new URLSearchParams(data)}).then(function (r) {
      if (!r.ok) throw new Error(r.status);
    });
  }

  var colors = {light: "#f4efe4", dark: "#181b20"};
  var cycle = {auto: "light", light: "dark", dark: "auto"};
  function applyTheme(theme) {
    document.documentElement.dataset.theme = theme;
    document.querySelectorAll(".theme-switch button[name=theme]").forEach(function (b) {
      b.setAttribute("aria-pressed", String(b.value === theme));
    });
    document.querySelectorAll(".theme-btn").forEach(function (b) {
      var label = b.getAttribute("data-label-" + theme);
      b.dataset.mode = theme;
      b.value = cycle[theme];
      b.setAttribute("aria-label", label);
      b.title = label;
    });
    document.querySelectorAll('meta[name="theme-color"]').forEach(function (m) { m.remove(); });
    (theme === "auto" ? ["light", "dark"] : [theme]).forEach(function (t) {
      var m = document.createElement("meta");
      m.name = "theme-color";
      m.content = colors[t];
      if (theme === "auto") m.media = "(prefers-color-scheme: " + t + ")";
      document.head.appendChild(m);
    });
    post("/theme", {theme: theme}).catch(function () { show(strings.msgNetwork); });
  }

  document.addEventListener("click", function (e) {
    var button = e.target.closest(".theme-btn, .theme-switch button[name=theme]");
    if (!button) return;
    e.preventDefault();
    applyTheme(button.value);
    if (button.classList.contains("theme-btn")) say(button.getAttribute("aria-label"));
  });

  document.addEventListener("submit", function (e) {
    var b = e.submitter;
    if (b && b.dataset.confirm && !confirm(b.dataset.confirm)) {
      e.preventDefault();
      return;
    }
    var form = e.target;
    if (!form.classList.contains("lang-switch") || !b || b.name !== "lang") return;
    e.preventDefault();
    post("/lang", {lang: b.value}).then(function () { location.reload(); }, function () {
      var input = document.createElement("input");
      input.type = "hidden";
      input.name = "lang";
      input.value = b.value;
      form.appendChild(input);
      form.submit();
    });
  });

  document.addEventListener("click", function (e) {
    document.querySelectorAll("details.lang-menu[open]").forEach(function (d) {
      if (!d.contains(e.target)) d.open = false;
    });
  });
  document.addEventListener("keydown", function (e) {
    if (e.key !== "Escape") return;
    document.querySelectorAll("details.lang-menu[open]").forEach(function (d) {
      d.open = false;
      d.querySelector("summary").focus();
    });
  });

  var fromBlank = -1;
  var checkedItem = "";
  function blanks(box) {
    return Array.prototype.slice.call(box.querySelectorAll('input[name="a"]'));
  }
  function blank(input) {
    return !/\S/.test(input.value);
  }
  document.addEventListener("submit", function (e) {
    var inputs = blanks(e.target);
    if (!inputs.length) return;
    if (!(e.submitter && e.submitter.name === "giveup") && inputs.every(function (i) { return i.readOnly || blank(i); })) {
      e.preventDefault();
      e.stopImmediatePropagation();
      return;
    }
    fromBlank = inputs.indexOf(document.activeElement);
    var item = e.target.closest("li.item");
    checkedItem = item ? item.id : "";
  }, true);
  window.kenaBlank = function (box) {
    var from = fromBlank;
    fromBlank = -1;
    var inputs = blanks(box);
    var target = box.querySelector('input[name="a"][aria-invalid="true"]');
    for (var k = 1; !target && k <= inputs.length; k++) {
      var el = inputs[(from + k) % inputs.length];
      if (!el.readOnly && blank(el)) target = el;
    }
    return target || box.querySelector('input[name="a"]:not([readonly])');
  };
  document.body.addEventListener("htmx:afterSettle", function () {
    if (!checkedItem) return;
    var item = document.getElementById(checkedItem);
    checkedItem = "";
    var el = item && window.kenaBlank(item);
    if (el) el.focus({preventScroll: true});
  });

  if ("serviceWorker" in navigator) {
    navigator.serviceWorker.register("/sw.js", {scope: "/"}).catch(function () {});
  }

  if (navigator.clipboard) {
    document.querySelectorAll(".copy-btn").forEach(function (btn) {
      btn.hidden = false;
      btn.addEventListener("click", function () {
        var el = document.getElementById(btn.dataset.copy);
        navigator.clipboard.writeText(el.textContent).then(function () {
          var original = btn.textContent;
          btn.textContent = strings.msgCopied;
          setTimeout(function () { btn.textContent = original; }, 1500);
        });
      });
    });
  }
})();
