(function () {
  "use strict";

  var button = document.getElementById("reveal-all");
  if (!button) return;

  var items = document.querySelectorAll("details");

  button.addEventListener("click", function () {
    var open = button.dataset.state !== "open";
    items.forEach(function (item) {
      item.open = open;
    });
    button.dataset.state = open ? "open" : "closed";
    button.textContent = open ? "Hide all answers" : "Reveal all answers";
  });
})();
