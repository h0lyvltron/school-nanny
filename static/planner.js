// Drag a lesson card in the week planner to move it to another day. Hold Ctrl
// (or Cmd) to leave the original in place and drop a copy, or drop onto one of
// a day's child chips to copy it for that child instead.
//
// Mouse and trackpad use HTML5 drag-and-drop. Tablets and phones do not, so a
// long-press lifts the same card and the finger finishes the drop. "Copy to
// this day" stands in for Ctrl on a touch screen, where modifier keys are gone.
//
// Every drop turns into an HTMX request, so the server stays the only thing
// that decides what a day looks like afterwards.
(function () {
    "use strict";

    var LONG_PRESS_MS = 420;
    var CANCEL_MOVE_PX = 12;

    var dragging = null;
    var highlighted = null;
    var press = null;
    var ghost = null;

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
            removeGhost();
        }
    }

    function removeGhost() {
        if (ghost && ghost.parentNode) {
            ghost.parentNode.removeChild(ghost);
        }
        ghost = null;
    }

    function placeGhost(x, y) {
        if (!ghost) {
            return;
        }
        ghost.style.transform = "translate(" + (x - 28) + "px, " + (y - 28) + "px)";
    }

    function makeGhost(card, x, y) {
        removeGhost();
        ghost = card.cloneNode(true);
        ghost.removeAttribute("id");
        ghost.classList.add("lesson-ghost");
        ghost.setAttribute("aria-hidden", "true");
        document.body.appendChild(ghost);
        placeGhost(x, y);
    }

    function dropTargetAt(x, y) {
        var under = document.elementFromPoint(x, y);
        var chip = closest(under, ".kid-target");
        if (chip) {
            return chip;
        }
        var copyDay = closest(under, ".day-copy-target");
        if (copyDay) {
            return copyDay;
        }
        return closest(under, ".day");
    }

    function commitDrop(target, copyOverride) {
        if (!dragging || !target) {
            return;
        }
        var chip = target.classList.contains("kid-target") ? target : null;
        var copyBtn = target.classList.contains("day-copy-target") ? target : null;
        var day = chip || copyBtn || (target.classList.contains("day") ? target : null);
        if (!day) {
            return;
        }

        var date = day.getAttribute("data-date");
        var copy = Boolean(chip) || Boolean(copyBtn) || Boolean(copyOverride);
        var kidID = chip ? chip.getAttribute("data-kid-id") : "";
        var dayEl = chip || copyBtn ? closest(day, ".day") || day : day;

        if (!copy && date === dragging.date) {
            return;
        }

        var values = {
            view: "planner",
            scheduled_on: date,
            kid_filter: dayEl.getAttribute("data-kid-filter") || "0"
        };
        if (kidID) {
            values.kid_id = kidID;
        }

        window.htmx.ajax("POST", "/lessons/" + dragging.id + (copy ? "/clone" : "/reschedule"), {
            target: "#day-" + date,
            swap: "outerHTML",
            values: values
        });
    }

    function clearPress() {
        if (press && press.timer) {
            window.clearTimeout(press.timer);
        }
        if (press && press.card) {
            press.card.classList.remove("is-pressing");
        }
        press = null;
    }

    function beginDragFromCard(card, x, y) {
        dragging = {
            id: card.getAttribute("data-lesson-id"),
            date: card.getAttribute("data-date"),
            card: card,
            touch: true
        };
        card.classList.add("is-dragging");
        setDragging(true);
        makeGhost(card, x, y);
        if (navigator.vibrate) {
            try { navigator.vibrate(12); } catch (e) { /* ignore */ }
        }
    }

    // Mouse / trackpad -------------------------------------------------

    document.addEventListener("dragstart", function (event) {
        if (event.pointerType === "touch") {
            return;
        }
        var card = closest(event.target, ".lesson[draggable='true']");
        if (!card) {
            return;
        }
        clearPress();
        dragging = {
            id: card.getAttribute("data-lesson-id"),
            date: card.getAttribute("data-date"),
            card: card,
            touch: false
        };
        card.classList.add("is-dragging");
        setDragging(true);
        if (event.dataTransfer) {
            event.dataTransfer.effectAllowed = "copyMove";
            event.dataTransfer.setData("text/plain", dragging.id);
        }
    });

    document.addEventListener("dragend", function () {
        if (dragging && dragging.card) {
            dragging.card.classList.remove("is-dragging");
        }
        dragging = null;
        setDragging(false);
    });

    document.addEventListener("dragover", function (event) {
        if (!dragging || dragging.touch) {
            return;
        }
        var chip = closest(event.target, ".kid-target");
        var copyBtn = closest(event.target, ".day-copy-target");
        var day = chip || copyBtn || closest(event.target, ".day");
        if (!day) {
            highlight(null);
            return;
        }
        event.preventDefault();
        if (event.dataTransfer) {
            event.dataTransfer.dropEffect = (chip || copyBtn || isCopy(event)) ? "copy" : "move";
        }
        highlight(chip || copyBtn || day);
    });

    document.addEventListener("dragleave", function (event) {
        if (highlighted && event.target === highlighted) {
            highlight(null);
        }
    });

    document.addEventListener("drop", function (event) {
        if (!dragging || dragging.touch) {
            return;
        }
        var chip = closest(event.target, ".kid-target");
        var copyBtn = closest(event.target, ".day-copy-target");
        var day = closest(event.target, ".day");
        if (!chip && !copyBtn && !day) {
            return;
        }
        event.preventDefault();
        commitDrop(chip || copyBtn || day, isCopy(event));
    });

    // Touch / stylus ---------------------------------------------------

    document.addEventListener("pointerdown", function (event) {
        if (event.pointerType === "mouse") {
            return;
        }
        var card = closest(event.target, ".lesson[draggable='true']");
        if (!card || event.button > 0) {
            return;
        }
        // Buttons and links on the card must keep working as taps.
        if (closest(event.target, "button, a, input, select, textarea, label, form")) {
            return;
        }

        clearPress();
        press = {
            card: card,
            pointerId: event.pointerId,
            x: event.clientX,
            y: event.clientY,
            timer: window.setTimeout(function () {
                if (!press || press.card !== card) {
                    return;
                }
                var x = press.x;
                var y = press.y;
                clearPress();
                beginDragFromCard(card, x, y);
                try { card.setPointerCapture(event.pointerId); } catch (e) { /* ignore */ }
            }, LONG_PRESS_MS)
        };
        card.classList.add("is-pressing");
    });

    document.addEventListener("pointermove", function (event) {
        if (press && press.pointerId === event.pointerId) {
            var dx = event.clientX - press.x;
            var dy = event.clientY - press.y;
            if ((dx * dx + dy * dy) > (CANCEL_MOVE_PX * CANCEL_MOVE_PX)) {
                // Finger moved — this is a scroll, not a drag.
                clearPress();
            }
            return;
        }

        if (!dragging || !dragging.touch || dragging.id === undefined) {
            return;
        }
        event.preventDefault();
        placeGhost(event.clientX, event.clientY);
        highlight(dropTargetAt(event.clientX, event.clientY));
    }, {passive: false});

    function endTouchDrag(event) {
        if (press && press.pointerId === event.pointerId) {
            clearPress();
            return;
        }
        if (!dragging || !dragging.touch) {
            return;
        }
        var target = dropTargetAt(event.clientX, event.clientY);
        var card = dragging.card;
        commitDrop(target, false);
        if (card) {
            card.classList.remove("is-dragging");
            try { card.releasePointerCapture(event.pointerId); } catch (e) { /* ignore */ }
        }
        dragging = null;
        setDragging(false);
    }

    document.addEventListener("pointerup", endTouchDrag);
    document.addEventListener("pointercancel", function (event) {
        if (press && press.pointerId === event.pointerId) {
            clearPress();
        }
        if (dragging && dragging.touch) {
            if (dragging.card) {
                dragging.card.classList.remove("is-dragging");
            }
            dragging = null;
            setDragging(false);
        }
    });
})();
