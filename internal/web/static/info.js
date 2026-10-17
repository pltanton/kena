(function () {
  var sheet = document.getElementById("info-sheet");
  if (!sheet) return;
  var slot = document.getElementById("info-card");
  var scroll = sheet.querySelector(".info-scroll");
  var general = sheet.querySelector(".info-general");

  function reveal(root) {
    root.querySelectorAll("[data-info][hidden], [data-open-info][hidden]").forEach(function (b) { b.hidden = false; });
  }
  if (window.htmx) htmx.onLoad(reveal);
  else reveal(document);

  function open(from) {
    var host = from.closest(".card, .item");
    var tpl = host && host.querySelector("template.info-rule");
    var key = tpl ? tpl.dataset.key : "";
    slot.replaceChildren();
    if (tpl) slot.appendChild(tpl.content.cloneNode(true));
    if (general) {
      var shown = 0;
      general.querySelectorAll("[data-key]").forEach(function (el) {
        el.hidden = el.dataset.key === key;
        if (!el.hidden) shown++;
      });
      general.hidden = shown === 0;
    }
    sheet.hidden = false;
    scroll.scrollTop = 0;
  }

  function close() {
    if (sheet.hidden) return;
    sheet.hidden = true;
    document.dispatchEvent(new CustomEvent("info:close"));
  }

  document.addEventListener("click", function (e) {
    var button = e.target.closest("[data-info], [data-open-info]");
    if (button) {
      e.preventDefault();
      open(button);
      return;
    }
    if (e.target.closest("[data-info-close]") || (!sheet.hidden && !sheet.contains(e.target))) close();
  });
  document.addEventListener("keydown", function (e) {
    if (e.key === "Escape" && !sheet.hidden) {
      e.preventDefault();
      close();
    }
  });
  document.body.addEventListener("htmx:afterSwap", close);
})();
