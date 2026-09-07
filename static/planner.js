// Drag a lesson card in the week planner to move it to another day. Hold Ctrl
// (or Cmd) to leave the original in place and drop a copy, or drop onto one of
// a day's child chips to copy it for that child instead.
//
// Every drop turns into an HTMX request, so the server stays the only thing
// that decides what a day looks like afterwards.
(function () {
    "use strict";

    var dragging = null;
    var highlighted = null;

    function isCopy(event) {
        return event.ctrlKey || event.metaKey;
    }

    function closest(node, selector) {
        if (!node) {
            return null;
        }
        if (node.nodeType !== 1) {
            node = node.parentElement;
        }
        return node ? node.closest(selector) : null;
    }

    function highlight(node) {
        if (highlighted === node) {
            return;
        }
        if (highlighted) {
            highlighted.classList.remove("is-drop-target");
        }
        highlighted = node;
        if (highlighted) {
            highlighted.classList.add("is-drop-target");
        }
    }

    // The chips only make sense mid-drag, so the planner is otherwise quiet.
    function setDragging(on) {
        document.body.classList.toggle("is-dragging-lesson", on);
        if (!on) {
            highlight(null);
        }
    }

    document.addEventListener("dragstart", function (event) {
        var card = closest(event.target, ".lesson[draggable='true']");
        if (!card) {
            return;
        }
        dragging = {
            id: card.getAttribute("data-lesson-id"),
            date: card.getAttribute("data-date"),
            card: card
        };
        card.classList.add("is-dragging");
        setDragging(true);
        if (event.dataTransfer) {
            event.dataTransfer.effectAllowed = "copyMove";
            // Firefox refuses to start a drag without some payload.
            event.dataTransfer.setData("text/plain", dragging.id);
        }
    });

    document.addEventListener("dragend", function () {
        if (dragging) {
            dragging.card.classList.remove("is-dragging");
        }
        dragging = null;
        setDragging(false);
    });

    document.addEventListener("dragover", function (event) {
        if (!dragging) {
            return;
        }
        var chip = closest(event.target, ".kid-target");
        var day = chip || closest(event.target, ".day");
        if (!day) {
            highlight(null);
            return;
        }
        event.preventDefault();
        if (event.dataTransfer) {
            event.dataTransfer.dropEffect = (chip || isCopy(event)) ? "copy" : "move";
        }
        highlight(chip || day);
    });

    document.addEventListener("dragleave", function (event) {
        if (highlighted && event.target === highlighted) {
            highlight(null);
        }
    });

    document.addEventListener("drop", function (event) {
        if (!dragging) {
            return;
        }
        var chip = closest(event.target, ".kid-target");
        var day = closest(event.target, ".day");
        if (!chip && !day) {
            return;
        }
        event.preventDefault();

        var date = chip ? chip.getAttribute("data-date") : day.getAttribute("data-date");
        var lessonID = dragging.id;
        var copy = Boolean(chip) || isCopy(event);
        var kidID = chip ? chip.getAttribute("data-kid-id") : "";

        // Nothing to do when a lesson lands back where it started, unless the
        // drop asked for a copy.
        if (!copy && date === dragging.date) {
            return;
        }

        var values = {
            view: "planner",
            scheduled_on: date,
            kid_filter: day ? (day.getAttribute("data-kid-filter") || "0") : "0"
        };
        if (kidID) {
            values.kid_id = kidID;
        }

        window.htmx.ajax("POST", "/lessons/" + lessonID + (copy ? "/clone" : "/reschedule"), {
            target: "#day-" + date,
            swap: "outerHTML",
            values: values
        });
    });
})();
