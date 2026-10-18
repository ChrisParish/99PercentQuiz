(function () {
  "use strict";

  var button = document.getElementById("reveal-all");
  if (button) {
    var items = document.querySelectorAll("details");

    button.addEventListener("click", function () {
      var open = button.dataset.state !== "open";
      items.forEach(function (item) {
        item.open = open;
      });
      button.dataset.state = open ? "open" : "closed";
      button.textContent = open ? "Hide all answers" : "Reveal all answers";
    });
  }

  var list = document.querySelector(".questions");
  var tally = document.getElementById("tally");
  if (list) {
    var storageKey = "quizMarks:" + list.dataset.date;

    var loadMarks = function () {
      try {
        return JSON.parse(window.localStorage.getItem(storageKey)) || {};
      } catch (e) {
        return {};
      }
    };

    var saveMarks = function (marks) {
      try {
        window.localStorage.setItem(storageKey, JSON.stringify(marks));
      } catch (e) {
        // Storage can be unavailable (e.g. Safari private browsing quota) -
        // marking still works for this page view, it just won't persist.
      }
    };

    var marks = loadMarks();
    var rows = list.querySelectorAll("li[data-id]");

    var applyMark = function (row, mark) {
      row.classList.toggle("marked-correct", mark === "correct");
      row.classList.toggle("marked-incorrect", mark === "incorrect");
      row.querySelectorAll(".mark-controls button").forEach(function (btn) {
        btn.setAttribute("aria-pressed", btn.dataset.mark === mark ? "true" : "false");
      });
    };

    var updateTally = function () {
      if (!tally) {
        return;
      }
      var correct = 0;
      rows.forEach(function (row) {
        if (marks[row.dataset.id] === "correct") {
          correct += 1;
        }
      });
      tally.textContent = correct + " / " + rows.length + " correct";
    };

    rows.forEach(function (row) {
      applyMark(row, marks[row.dataset.id]);

      row.querySelectorAll(".mark-controls button").forEach(function (btn) {
        btn.addEventListener("click", function () {
          var mark = btn.dataset.mark;
          var id = row.dataset.id;
          marks[id] = marks[id] === mark ? undefined : mark;
          if (marks[id] === undefined) {
            delete marks[id];
          }
          applyMark(row, marks[id]);
          saveMarks(marks);
          updateTally();
        });
      });
    });

    updateTally();
  }

  // Swap replaces the question for every visitor to this date, not just this
  // tap - confirm first since it sits right below the mark buttons and is
  // easy to hit by mistake.
  document.querySelectorAll(".swap-form").forEach(function (form) {
    form.addEventListener("submit", function (event) {
      if (!window.confirm("Swap this question for a different one? This replaces it for everyone viewing this date.")) {
        event.preventDefault();
      }
    });
  });
})();
