document.querySelectorAll(".pass").forEach(function (box) {
  var input = box.querySelector("input");
  var button = box.querySelector(".reveal");
  function set(show) {
    input.type = show ? "text" : "password";
    button.textContent = show ? "Hide" : "Show";
    button.setAttribute("aria-pressed", String(show));
  }
  button.hidden = false;
  button.addEventListener("click", function () {
    set(input.type === "password");
    input.focus();
  });
  if (input.form) input.form.addEventListener("submit", function () { set(false); });
});
