// Pick a day on a month calendar, or drag across several for something that
// lasts longer than one: a trip, a stretch of appointments, a week away.
//
// A plain click is a link, so the calendar still works with scripting off.
// This file adds the dragging on top and commits by navigating to the same
// address the link would have used, which keeps the server the only thing
// deciding what the page says afterwards.
//
// On a touch screen a swipe has to stay a scroll, so a press has to be held
// before it turns into a selection - the same bargain the planner makes with
// its cards.
(function () {
    "use strict";

    var LONG_PRESS_MS = 420;
    var CANCEL_MOVE_PX = 12;
    var DRAG_START_PX = 6;

    var press = null;
    var drag = null;
    var swallowClick = false;

    function closest(node, selector) {
        if (!node) {
            return null;
        }
        if (node.nodeType !== 1) {
            node = node.parentElement;
        }
        return node ? node.closest(selector) : null;
    }

    function cellAt(x, y, grid) {
        var cell = closest(document.elementFromPoint(x, y), ".cal-cell[data-date]");
        if (!cell || (grid && closest(cell, "[data-calendar]") !== grid)) {
            return null;
        }
        return cell;
    }

    // Paint the run as the pointer moves so the stretch being chosen is
    // visible before it is committed.
    function paint(grid, from, to) {
        if (to < from) {
            var swap = from;
            from = to;
            to = swap;
        }
        var cells = grid.querySelectorAll(".cal-cell[data-date]");
        for (var i = 0; i < cells.length; i++) {
            var date = cells[i].getAttribute("data-date");
            cells[i].classList.toggle("is-selected", date >= from && date <= to);
        }
    }

    function commit(grid, from, to) {
        if (to < from) {
            var swap = from;
            from = to;
            to = swap;
        }
        window.location.href = grid.getAttribute("data-url") +
            "?month=" + encodeURIComponent(grid.getAttribute("data-month")) +
            "&from=" + encodeURIComponent(from) +
            "&to=" + encodeURIComponent(to);
    }

    function clearPress() {
        if (press && press.timer) {
            window.clearTimeout(press.timer);
        }
        press = null;
    }

    function beginDrag(cell, pointerId) {
        var grid = closest(cell, "[data-calendar]");
        if (!grid) {
            return;
        }
        drag = {
            grid: grid,
            from: cell.getAttribute("data-date"),
            to: cell.getAttribute("data-date"),
            pointerId: pointerId
        };
        grid.classList.add("is-selecting");
        paint(grid, drag.from, drag.to);
        try { cell.setPointerCapture(pointerId); } catch (e) { /* ignore */ }
    }

    function endDrag() {
        if (drag) {
            drag.grid.classList.remove("is-selecting");
        }
        drag = null;
    }

    // The date in each cell is a link, and a browser drags links by itself.
    // Left alone that turns the start of a selection into the ghost of a URL
    // being dragged around the page.
    document.addEventListener("dragstart", function (event) {
        if (closest(event.target, ".cal-cell[data-date]")) {
            event.preventDefault();
        }
    });

    document.addEventListener("pointerdown", function (event) {
        if (event.button > 0) {
            return;
        }
        var cell = closest(event.target, ".cal-cell[data-date]");
        if (!cell || !closest(cell, "[data-calendar]")) {
            return;
        }
        // Anything else in the cell keeps behaving like itself.
        if (closest(event.target, "button, input, select, textarea, form")) {
            return;
        }

        clearPress();
        press = {
            cell: cell,
            pointerId: event.pointerId,
            touch: event.pointerType !== "mouse",
            x: event.clientX,
            y: event.clientY,
            timer: null
        };
        if (press.touch) {
            // Hold to select; a quick swipe is the page scrolling instead.
            press.timer = window.setTimeout(function () {
                if (!press || press.cell !== cell) {
                    return;
                }
                var pointerId = press.pointerId;
                clearPress();
                beginDrag(cell, pointerId);
                if (navigator.vibrate) {
                    try { navigator.vibrate(12); } catch (e) { /* ignore */ }
                }
            }, LONG_PRESS_MS);
        }
    });

    document.addEventListener("pointermove", function (event) {
        if (press && press.pointerId === event.pointerId) {
            var dx = event.clientX - press.x;
            var dy = event.clientY - press.y;
            var moved = (dx * dx + dy * dy);
            if (press.touch) {
                if (moved > (CANCEL_MOVE_PX * CANCEL_MOVE_PX)) {
                    clearPress();
                }
            } else if (moved > (DRAG_START_PX * DRAG_START_PX)) {
                var cell = press.cell;
                var pointerId = press.pointerId;
                clearPress();
                beginDrag(cell, pointerId);
            }
            return;
        }

        if (!drag || drag.pointerId !== event.pointerId) {
            return;
        }
        // The gesture belongs to the calendar now, so keep it off the page.
        event.preventDefault();
        var over = cellAt(event.clientX, event.clientY, drag.grid);
        if (over) {
            drag.to = over.getAttribute("data-date");
            paint(drag.grid, drag.from, drag.to);
        }
    }, {passive: false});

    document.addEventListener("pointerup", function (event) {
        if (press && press.pointerId === event.pointerId) {
            clearPress();
            return;
        }
        if (!drag || drag.pointerId !== event.pointerId) {
            return;
        }
        var grid = drag.grid;
        var from = drag.from;
        var to = drag.to;
        endDrag();
        swallowClick = true;
        commit(grid, from, to);
    });

    document.addEventListener("pointercancel", function (event) {
        if (press && press.pointerId === event.pointerId) {
            clearPress();
        }
        if (drag && drag.pointerId === event.pointerId) {
            endDrag();
        }
    });

    // A click anywhere in a cell selects that day, not only the date itself.
    document.addEventListener("click", function (event) {
        var cell = closest(event.target, ".cal-cell[data-date]");
        if (!cell) {
            return;
        }
        var grid = closest(cell, "[data-calendar]");
        if (!grid) {
            return;
        }
        if (swallowClick) {
            // The drag already decided where to go.
            swallowClick = false;
            event.preventDefault();
            return;
        }
        if (closest(event.target, "a, button, input, select, textarea, form")) {
            return;
        }
        event.preventDefault();
        var date = cell.getAttribute("data-date");
        commit(grid, date, date);
    });
})();
