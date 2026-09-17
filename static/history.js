(function () {
    "use strict";

    function isEditing(target) {
        if (!target) {
            return false;
        }
        var tag = (target.tagName || "").toLowerCase();
        return tag === "input" || tag === "textarea" || tag === "select" ||
            target.isContentEditable;
    }

    function walk(action) {
        var form = document.querySelector('form[action="/history/' + action + '"]');
        if (!form) {
            return;
        }
        var button = form.querySelector("button");
        if (button && !button.disabled) {
            if (form.requestSubmit) {
                form.requestSubmit(button);
            } else {
                form.submit();
            }
        }
    }

    document.addEventListener("keydown", function (event) {
        if (!event.ctrlKey || !event.shiftKey || event.altKey || event.metaKey ||
            isEditing(event.target)) {
            return;
        }
        var key = (event.key || "").toLowerCase();
        if (key === "z") {
            event.preventDefault();
            walk("undo");
        } else if (key === "y") {
            event.preventDefault();
            walk("redo");
        }
    });

    document.addEventListener("htmx:afterRequest", function (event) {
        var xhr = event.detail && event.detail.xhr;
        if (!xhr) {
            return;
        }
        var current = xhr.getResponseHeader("X-School-Nanny-History");
        if (current === null || current === "") {
            return;
        }
        document.querySelectorAll('form[action^="/history/"] input[name="expected"]').forEach(function (input) {
            input.value = current;
        });
        var revision = xhr.getResponseHeader("X-School-Nanny-History-Revision");
        if (revision !== null && revision !== "") {
            document.querySelectorAll('form[action^="/history/"] input[name="expected_revision"]').forEach(function (input) {
                input.value = revision;
            });
        }
        var undo = document.querySelector('form[action="/history/undo"] button');
        if (undo) {
            undo.disabled = false;
        }
        var toast = document.createElement("div");
        toast.className = "save-toast";
        toast.setAttribute("role", "status");
        toast.textContent = "Saved — Ctrl+Shift+Z to undo";
        document.body.appendChild(toast);
        window.setTimeout(function () {
            toast.classList.add("is-leaving");
            window.setTimeout(function () { toast.remove(); }, 400);
        }, 1800);
    });
})();
