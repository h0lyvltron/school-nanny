// Drag a lesson card in the week planner to move it to another day. Hold Ctrl
// (or Cmd) to leave the original in place and drop a copy, or drop onto one of
// a day's child chips to copy it for that child instead.
//
// Mouse, trackpad, tablets, and phones all use a pointer ghost so the week can
// swap under the card. Tablets and phones wait for a long-press before lifting
// the card, so a finger can still scroll the page. "Copy to this day" stands in
// for Ctrl on a touch screen, where modifier keys are gone.
//
// Hovering the left or right edge, or Previous / Next, loads the adjacent week
// without dropping the card. Every drop turns into an HTMX request, so the
// server stays the only thing that decides what a day looks like afterwards.
(function () {
    "use strict";

    var LONG_PRESS_MS = 420;
    var CANCEL_MOVE_PX = 12;
    var DRAG_START_PX = 8;
    var WEEK_SHIFT_MS = 600;
    var EDGE_PX = 48;
    var EDGE_PX_NARROW = 28;

    var dragging = null;
    var highlighted = null;
    var press = null;
    var ghost = null;
    var weekShift = null;
    var shiftingWeek = false;
    var lastX = 0;
    var lastY = 0;
    var suppressClick = false;

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

    function setDragging(on) {
        document.body.classList.toggle("is-dragging-lesson", on);
        if (!on) {
            highlight(null);
            removeGhost();
            clearWeekShift();
            document.body.classList.remove("is-shifting-week-prev", "is-shifting-week-next");
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
        if (closest(under, ".week-nav")) {
            return null;
        }
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

    function beginDragFromCard(card, x, y, fromTouch) {
        dragging = {
            id: card.getAttribute("data-lesson-id"),
            date: card.getAttribute("data-date"),
            card: card,
            touch: Boolean(fromTouch)
        };
        card.classList.add("is-dragging");
        setDragging(true);
        makeGhost(card, x, y);
        if (fromTouch && navigator.vibrate) {
            try { navigator.vibrate(12); } catch (e) { /* ignore */ }
        }
    }

    function finishDrag(event) {
        if (!dragging) {
            return;
        }
        var target = dropTargetAt(event.clientX, event.clientY);
        var card = dragging.card;
        var copy = dragging.touch ? false : isCopy(event);
        commitDrop(target, copy);
        if (card) {
            card.classList.remove("is-dragging");
            try { card.releasePointerCapture(event.pointerId); } catch (e) { /* ignore */ }
        }
        try { document.body.releasePointerCapture(event.pointerId); } catch (e) { /* ignore */ }
        dragging = null;
        suppressClick = true;
        setDragging(false);
    }

    function capturePointer(event) {
        try { document.body.setPointerCapture(event.pointerId); } catch (e) {
            try { event.target.setPointerCapture(event.pointerId); } catch (err) { /* ignore */ }
        }
    }

    // Native HTML5 drag would cancel if the origin card is swapped out with the
    // week, so lesson cards keep draggable="true" only for the grab cursor.
    document.addEventListener("dragstart", function (event) {
        if (closest(event.target, ".lesson[draggable='true']")) {
            event.preventDefault();
        }
    });

    document.addEventListener("click", function (event) {
        if (!suppressClick) {
            return;
        }
        suppressClick = false;
        event.preventDefault();
        event.stopPropagation();
    }, true);

    // Mouse / trackpad / touch -----------------------------------------

    document.addEventListener("pointerdown", function (event) {
        var card = closest(event.target, ".lesson[draggable='true']");
        if (!card || event.button > 0) {
            return;
        }
        if (closest(event.target, "button, a, input, select, textarea, label, form")) {
            return;
        }

        clearPress();
        lastX = event.clientX;
        lastY = event.clientY;

        if (event.pointerType === "mouse") {
            press = {
                card: card,
                pointerId: event.pointerId,
                x: event.clientX,
                y: event.clientY,
                mouse: true,
                timer: null
            };
            event.preventDefault();
            return;
        }

        press = {
            card: card,
            pointerId: event.pointerId,
            x: event.clientX,
            y: event.clientY,
            mouse: false,
            timer: window.setTimeout(function () {
                if (!press || press.card !== card) {
                    return;
                }
                var x = press.x;
                var y = press.y;
                clearPress();
                beginDragFromCard(card, x, y, true);
                capturePointer(event);
            }, LONG_PRESS_MS)
        };
        card.classList.add("is-pressing");
    });

    document.addEventListener("pointermove", function (event) {
        lastX = event.clientX;
        lastY = event.clientY;

        if (press && press.pointerId === event.pointerId) {
            var dx = event.clientX - press.x;
            var dy = event.clientY - press.y;
            var dist = (dx * dx) + (dy * dy);
            if (press.mouse) {
                if (dist > (DRAG_START_PX * DRAG_START_PX)) {
                    var card = press.card;
                    var pointerEvent = event;
                    clearPress();
                    beginDragFromCard(card, pointerEvent.clientX, pointerEvent.clientY, false);
                    capturePointer(pointerEvent);
                }
                return;
            }
            if (dist > (CANCEL_MOVE_PX * CANCEL_MOVE_PX)) {
                clearPress();
            }
            return;
        }

        if (!dragging) {
            return;
        }
        event.preventDefault();
        placeGhost(event.clientX, event.clientY);
        highlight(dropTargetAt(event.clientX, event.clientY));
        considerWeekShift(event.clientX, event.clientY);
    }, {passive: false});

    document.addEventListener("pointerup", function (event) {
        if (press && press.pointerId === event.pointerId) {
            clearPress();
            return;
        }
        if (dragging) {
            finishDrag(event);
        }
    });

    document.addEventListener("pointercancel", function (event) {
        if (press && press.pointerId === event.pointerId) {
            clearPress();
        }
        if (dragging) {
            if (dragging.card) {
                dragging.card.classList.remove("is-dragging");
            }
            try { document.body.releasePointerCapture(event.pointerId); } catch (e) { /* ignore */ }
            dragging = null;
            setDragging(false);
        }
    });

    // Adjacent week while dragging -------------------------------------

    function edgeWidth() {
        return window.innerWidth < 640 ? EDGE_PX_NARROW : EDGE_PX;
    }

    function weekNavLink(dir) {
        return document.querySelector('#planner-week .week-nav a[data-week-shift="' + dir + '"]');
    }

    function paintWeekShift(dir) {
        document.body.classList.toggle("is-shifting-week-prev", dir === "prev");
        document.body.classList.toggle("is-shifting-week-next", dir === "next");
        var links = document.querySelectorAll("#planner-week .week-nav a[data-week-shift]");
        for (var i = 0; i < links.length; i++) {
            links[i].classList.toggle("is-week-shift", links[i].getAttribute("data-week-shift") === dir);
        }
    }

    function clearWeekShift() {
        if (weekShift && weekShift.timer) {
            window.clearTimeout(weekShift.timer);
        }
        weekShift = null;
        paintWeekShift("");
    }

    function shiftTargetAt(x, y) {
        var under = document.elementFromPoint(x, y);
        var link = closest(under, "#planner-week .week-nav a[data-week-shift]");
        if (link) {
            var dir = link.getAttribute("data-week-shift");
            if (dir === "prev" || dir === "next") {
                return {dir: dir, link: link};
            }
            return null;
        }
        var edge = edgeWidth();
        if (x <= edge) {
            return {dir: "prev", link: weekNavLink("prev")};
        }
        if (x >= window.innerWidth - edge) {
            return {dir: "next", link: weekNavLink("next")};
        }
        return null;
    }

    function loadWeek(link) {
        if (!link || !window.htmx || shiftingWeek) {
            return;
        }
        var url = link.getAttribute("href");
        if (!url) {
            return;
        }
        shiftingWeek = true;
        clearWeekShift();
        window.htmx.ajax("GET", url, {
            source: link,
            target: "#planner-week",
            select: "#planner-week",
            swap: "outerHTML"
        });
    }

    function considerWeekShift(x, y) {
        if (!dragging || shiftingWeek) {
            return;
        }
        var target = shiftTargetAt(x, y);
        if (!target || !target.link) {
            clearWeekShift();
            return;
        }
        if (weekShift && weekShift.dir === target.dir) {
            return;
        }
        clearWeekShift();
        paintWeekShift(target.dir);
        weekShift = {
            dir: target.dir,
            link: target.link,
            timer: window.setTimeout(function () {
                var pending = weekShift;
                weekShift = null;
                if (pending && pending.link) {
                    loadWeek(pending.link);
                }
            }, WEEK_SHIFT_MS)
        };
    }

    function plannerSwapTarget(node) {
        return Boolean(node && node.id === "planner-week");
    }

    function onPlannerWeekReady() {
        applyEventsToggle(eventsVisible());
        shiftingWeek = false;
        if (dragging) {
            var grid = document.querySelector("#planner-week .week-grid");
            if (grid) {
                afterPaint(function () {
                    scrollBoardTo(grid, boardScrollMargin());
                });
            }
            considerWeekShift(lastX, lastY);
            return;
        }
        afterPaint(scrollPlannerIntoPlace);
    }

    function boardScrollMargin() {
        return 8;
    }

    document.body.addEventListener("htmx:afterSwap", function (event) {
        var elt = event.detail && event.detail.elt;
        if (plannerSwapTarget(elt)) {
            onPlannerWeekReady();
        }
    });

    document.body.addEventListener("htmx:responseError", function () {
        shiftingWeek = false;
    });
    document.body.addEventListener("htmx:sendError", function () {
        shiftingWeek = false;
    });
    document.body.addEventListener("htmx:timeout", function () {
        shiftingWeek = false;
    });

    // Scroll the current day into view after the week has painted. iPad
    // Safari's scrollIntoView also moves the window, which draws today over
    // the app bar for a frame; move only the board scroller, and only then.
    function boardScroller(from) {
        return from && from.closest ? from.closest(".page-board-scroll") : null;
    }

    function scrollBoardTo(el, margin) {
        if (!el) {
            return;
        }
        var scroller = boardScroller(el);
        if (!scroller) {
            return;
        }
        var sBox = scroller.getBoundingClientRect();
        var eBox = el.getBoundingClientRect();
        var next = scroller.scrollTop + (eBox.top - sBox.top) - (margin || 0);
        if (next < 0) {
            next = 0;
        }
        scroller.scrollTop = next;
    }

    function afterPaint(fn) {
        window.requestAnimationFrame(function () {
            window.requestAnimationFrame(fn);
        });
    }

    function scrollPlannerIntoPlace() {
        function place(withFocus) {
            var root = document.getElementById("planner-week");
            if (!root) {
                return;
            }
            var margin = boardScrollMargin();
            var today = root.querySelector(".day.is-today");
            if (today) {
                scrollBoardTo(today, margin);
                if (withFocus) {
                    today.setAttribute("tabindex", "-1");
                    try { today.focus({preventScroll: true}); } catch (e) { /* ignore */ }
                }
                return;
            }
            var grid = root.querySelector(".week-grid");
            if (grid) {
                scrollBoardTo(grid, margin);
            }
        }
        place(true);
        window.requestAnimationFrame(function () {
            place(false);
        });
    }

    // Calendar events on the week --------------------------------------

    var EVENTS_KEY = "school-nanny-week-events";

    function eventsFromQuery() {
        try {
            var raw = new URLSearchParams(window.location.search).get("events");
            if (raw === "1" || raw === "on" || raw === "true") {
                return true;
            }
            if (raw === "0" || raw === "off" || raw === "false") {
                return false;
            }
        } catch (e) {
            // Malformed search strings should not hide the week.
        }
        return null;
    }

    function rememberEvents(on) {
        try {
            localStorage.setItem(EVENTS_KEY, on ? "1" : "0");
        } catch (e) {
            // Private windows can block storage; the toggle still works for this page.
        }
    }

    function eventsVisible() {
        var fromQuery = eventsFromQuery();
        if (fromQuery !== null) {
            rememberEvents(fromQuery);
            return fromQuery;
        }
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
    afterPaint(scrollPlannerIntoPlace);

    document.addEventListener("change", function (event) {
        var input = event.target;
        if (!input || !input.matches || !input.matches("[data-events-toggle]")) {
            return;
        }
        var on = Boolean(input.checked);
        rememberEvents(on);
        applyEventsToggle(on);
    });
})();
