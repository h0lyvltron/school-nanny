// Appearance: light/dark mode plus this computer's look (nav, palette, cards).
//
// The document head has already resolved and applied the attributes by the
// time this runs. This file handles the theme button, the Settings radios,
// and keeps "automatic" honest when the computer changes its mind.
(function () {
    "use strict";

    var STORAGE_KEY = "school-nanny-theme";
    var MODES = ["auto", "light", "dark"];
    var LABELS = { auto: "Auto", light: "Light", dark: "Dark" };
    var LOOK = {
        nav: { key: "school-nanny-nav", attr: "data-nav", values: ["underline", "tabs", "pills"], fallback: "underline" },
        palette: { key: "school-nanny-palette", attr: "data-palette", values: ["warm", "cool", "contrast"], fallback: "warm" },
        cards: { key: "school-nanny-cards", attr: "data-cards", values: ["plain", "folder", "bubble"], fallback: "plain" }
    };

    var root = document.documentElement;
    var prefersDark = window.matchMedia("(prefers-color-scheme: dark)");
    var mode = readMode();

    function readMode() {
        var saved = null;
        try {
            saved = localStorage.getItem(STORAGE_KEY);
        } catch (e) {
            // Storage can be unavailable in private windows; automatic is fine.
        }
        return saved === "light" || saved === "dark" ? saved : "auto";
    }

    function saveMode(next) {
        try {
            if (next === "auto") {
                localStorage.removeItem(STORAGE_KEY);
            } else {
                localStorage.setItem(STORAGE_KEY, next);
            }
        } catch (e) {
            // Not being able to remember the choice should not break the page.
        }
    }

    function allowed(spec, value) {
        return spec.values.indexOf(value) >= 0 ? value : spec.fallback;
    }

    function readLook(spec) {
        var saved = null;
        try {
            saved = localStorage.getItem(spec.key);
        } catch (e) {
            // Private windows keep the defaults from the document head.
        }
        return allowed(spec, saved);
    }

    function saveLook(spec, value) {
        try {
            localStorage.setItem(spec.key, value);
        } catch (e) {
            // Not being able to remember the choice should not break the page.
        }
    }

    function apply(next) {
        var dark = next === "dark" || (next === "auto" && prefersDark.matches);
        root.setAttribute("data-theme", dark ? "dark" : "light");
        root.setAttribute("data-theme-mode", next);

        var buttons = document.querySelectorAll("[data-theme-toggle]");
        for (var i = 0; i < buttons.length; i++) {
            buttons[i].setAttribute("data-mode", next);
            var label = buttons[i].querySelector("[data-theme-label]");
            if (label) {
                label.textContent = LABELS[next];
            }
        }
    }

    function applyLook() {
        Object.keys(LOOK).forEach(function (kind) {
            var spec = LOOK[kind];
            var value = readLook(spec);
            root.setAttribute(spec.attr, value);
            var inputs = document.querySelectorAll('input[data-look="' + kind + '"]');
            for (var i = 0; i < inputs.length; i++) {
                inputs[i].checked = inputs[i].value === value;
            }
        });
    }

    apply(mode);
    applyLook();

    // Delegated so the button keeps working after HTMX replaces part of a page.
    document.addEventListener("click", function (event) {
        var button = event.target.closest && event.target.closest("[data-theme-toggle]");
        if (!button) {
            return;
        }
        mode = MODES[(MODES.indexOf(mode) + 1) % MODES.length];
        saveMode(mode);
        apply(mode);
    });

    document.addEventListener("change", function (event) {
        var input = event.target;
        if (!input || !input.getAttribute) {
            return;
        }
        var kind = input.getAttribute("data-look");
        var spec = kind && LOOK[kind];
        if (!spec) {
            return;
        }
        var value = allowed(spec, input.value);
        saveLook(spec, value);
        root.setAttribute(spec.attr, value);
    });

    document.addEventListener("htmx:afterSwap", applyLook);

    function followSystem() {
        if (mode === "auto") {
            apply("auto");
        }
    }

    if (prefersDark.addEventListener) {
        prefersDark.addEventListener("change", followSystem);
    } else if (prefersDark.addListener) {
        prefersDark.addListener(followSystem);
    }
})();
