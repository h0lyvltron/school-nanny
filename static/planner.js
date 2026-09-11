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

    // The name a day goes by on screen, so a question about it reads the way
    // the week does: "Monday 28 Sep".
    function dayLabel(date) {
        var day = document.getElementById("day-" + date);
        if (!day) {
            return date;
        }
        var name = day.querySelector(".day-head h3");
        var number = day.querySelector(".day-date");
        return [name, number].filter(Boolean).map(function (node) {
            return node.textContent.trim();
        }).join(" ") || date;
    }

    function lessonTitle(card) {
        var title = card.querySelector(".lesson-title");
        return title ? title.textContent.trim() : "That lesson";
    }

    function sentence(list) {
        if (list.length < 2) {
            return list.join("");
        }
        return list.slice(0, -1).join(", ") + " and " + list[list.length - 1];
    }

    // Dragging a lesson from a curriculum set back past work that is still
    // pending is not the simple day swap it looks like. The lesson shares the
    // day it lands on, and everything still ahead of it closes up behind, one
    // school day each. That rearranges days she is not even pointing at, so
    // she gets told what it will do before it happens.
    function shuffleWarning(card, date) {
        var assignment = card.getAttribute("data-assignment-id");
        if (!assignment || !date || date >= card.getAttribute("data-date")) {
            return "";
        }

        var sequence = Number(card.getAttribute("data-sequence"));
        var siblings = document.querySelectorAll(
            ".lesson.status-planned[data-assignment-id='" + assignment + "']");
        var sharing = [];
        var following = 0;
        var jumped = 0;
        for (var i = 0; i < siblings.length; i++) {
            var other = siblings[i];
            var on = other.getAttribute("data-date");
            if (other === card || on < date) {
                continue;
            }
            if (Number(other.getAttribute("data-sequence")) < sequence) {
                jumped++;
            }
            if (on === date) {
                sharing.push(lessonTitle(other));
            } else {
                following++;
            }
        }
        if (!jumped) {
            return "";
        }

        var parts = [lessonTitle(card) + " belongs to a curriculum set."];
        if (sharing.length) {
            parts.push("It will share " + dayLabel(date) + " with " +
                sentence(sharing) + ".");
        }
        if (following) {
            parts.push("The " + following + " lesson" + (following === 1 ? "" : "s") +
                " still ahead of it will close up behind, one school day each.");
        }
        parts.push("Move it?");
        return parts.join(" ");
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
        if (!copy) {
            var warning = shuffleWarning(dragging.card, date);
            if (warning && !window.confirm(warning)) {
                return;
            }
        }

        var values = {
            view: "planner",
            scheduled_on: date,
            kid_filter: dayEl.getAttribute("data-kid-filter") || "0",
            adult_filter: dayEl.getAttribute("data-adult-filter") || "0"
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

    // Calendar events on the week --------------------------------------

    var EVENTS_KEY = "school-nanny-week-events";

    function eventsVisible() {
        try {
            var raw = localStorage.getItem(EVENTS_KEY);
            if (raw === null) {
                return true;
            }
            return raw !== "0" && raw !== "false";
        } catch (e) {
            return true;
        }
    }

    function applyEventsToggle(on) {
        var grid = document.querySelector("[data-week-events]");
        if (grid) {
            grid.classList.toggle("hide-day-events", !on);
        }
        var input = document.querySelector("[data-events-toggle]");
        if (input) {
            input.checked = on;
            input.setAttribute("aria-checked", on ? "true" : "false");
        }
    }

    applyEventsToggle(eventsVisible());

    document.addEventListener("change", function (event) {
        var input = event.target;
        if (!input || !input.matches || !input.matches("[data-events-toggle]")) {
            return;
        }
        var on = Boolean(input.checked);
        try {
            localStorage.setItem(EVENTS_KEY, on ? "1" : "0");
        } catch (e) {
            // Private windows can block storage; the toggle still works for this page.
        }
        applyEventsToggle(on);
    });
})();
