(function () {
  var panel = null;
  var owner = null;
  var gutter = 12;

  function choice(el) {
    return el && el.closest && el.closest(".gap select:not([disabled])");
  }

  function close(refocus) {
    if (!panel) return;
    panel.remove();
    panel = null;
    if (owner) owner.removeAttribute("aria-expanded");
    if (refocus && owner) owner.focus({preventScroll: true});
    owner = null;
  }

  function place() {
    var r = owner.getBoundingClientRect();
    var vw = document.documentElement.clientWidth;
    var vh = window.innerHeight;
    var width = Math.min(Math.max(r.width, 176), vw - 2 * gutter);
    var left = Math.min(Math.max(r.left + r.width / 2 - width / 2, gutter), vw - gutter - width);
    panel.style.width = width + "px";
    panel.style.left = left + "px";
    var h = panel.offsetHeight;
    var below = vh - r.bottom - gutter;
    var top = below >= h || below >= r.top - gutter ? r.bottom + 6 : r.top - 6 - h;
    panel.style.top = Math.max(gutter, top) + "px";
  }

  function pick(sel, value) {
    sel.value = value;
    close(true);
    sel.dispatchEvent(new Event("change", {bubbles: true}));
  }

  function items() {
    return Array.prototype.slice.call(panel.querySelectorAll("button:not([disabled])"));
  }

  function open(sel, byKey) {
    close(false);
    owner = sel;
    panel = document.createElement("div");
    panel.className = "picker";
    panel.tabIndex = -1;
    panel.setAttribute("role", "listbox");
    panel.setAttribute("lang", "el");
    panel.setAttribute("aria-label", sel.getAttribute("aria-label") || "");
    Array.prototype.forEach.call(sel.options, function (o) {
      if (o.value === "") return;
      var b = document.createElement("button");
      b.type = "button";
      b.setAttribute("role", "option");
      b.className = "picker-option";
      b.textContent = o.textContent;
      b.dataset.value = o.value;
      if (o.disabled) {
        b.disabled = true;
        b.classList.add("struck");
      }
      if (o.selected) b.setAttribute("aria-selected", "true");
      panel.appendChild(b);
    });
    document.body.appendChild(panel);
    sel.setAttribute("aria-expanded", "true");
    place();
    var first = panel.querySelector('button[aria-selected="true"]:not([disabled])') || items()[0];
    if (byKey && first) first.focus({preventScroll: true});
    else panel.focus({preventScroll: true});
  }

  ["pointerdown", "mousedown"].forEach(function (type) {
    document.addEventListener(type, function (e) {
      if (choice(e.target)) {
        e.preventDefault();
        return;
      }
      if (panel && !panel.contains(e.target)) close(false);
    }, true);
  });

  document.addEventListener("click", function (e) {
    var sel = choice(e.target);
    if (sel) {
      e.preventDefault();
      if (owner === sel) close(true);
      else open(sel, false);
      return;
    }
    var b = panel && e.target.closest(".picker-option");
    if (b && !b.disabled) pick(owner, b.dataset.value);
  });

  document.addEventListener("keydown", function (e) {
    var sel = choice(e.target);
    if (sel && !panel && (e.key === "Enter" || e.key === " " || e.key === "ArrowDown" || e.key === "ArrowUp")) {
      e.preventDefault();
      e.stopPropagation();
      open(sel, true);
      return;
    }
    if (!panel || !panel.contains(e.target)) return;
    var list = items();
    var i = list.indexOf(e.target);
    if (e.key === "Escape") {
      e.preventDefault();
      e.stopPropagation();
      close(true);
    } else if (e.key === "ArrowDown" || e.key === "ArrowUp") {
      e.preventDefault();
      var down = e.key === "ArrowDown";
      var next = i < 0 ? list[down ? 0 : list.length - 1] : list[(i + (down ? 1 : list.length - 1)) % list.length];
      if (next) next.focus({preventScroll: true});
    } else if (e.key === "Tab") {
      close(true);
    }
  }, true);

  function follow() {
    if (panel) place();
  }
  window.addEventListener("resize", follow);
  window.addEventListener("scroll", follow, true);
  if (window.visualViewport) window.visualViewport.addEventListener("resize", follow);
  document.body.addEventListener("htmx:beforeSwap", function () { close(false); });
})();
