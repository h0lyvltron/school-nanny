import * as pdfjsLib from "/static/pdfjs/pdf.min.mjs";

pdfjsLib.GlobalWorkerOptions.workerSrc = "/static/pdfjs/pdf.worker.min.mjs";

function clamp(n, lo, hi) {
  return Math.max(lo, Math.min(hi, n));
}

function parseIntAttr(el, name, fallback) {
  var raw = el.getAttribute(name);
  if (raw == null || raw === "") {
    return fallback;
  }
  var n = parseInt(raw, 10);
  return Number.isFinite(n) ? n : fallback;
}

async function mountViewer(root) {
  var url = root.getAttribute("data-file-url");
  if (!url) {
    return;
  }

  var canvas = root.querySelector("[data-pdf-canvas]");
  var stage = root.querySelector("[data-pdf-stage]");
  var status = root.querySelector("[data-pdf-status]");
  var pageLabel = root.querySelector("[data-pdf-page-label]");
  if (!canvas || !stage) {
    return;
  }
  var ctx = canvas.getContext("2d");

  var fitMode = "width"; // width | page | custom
  var customScale = 1;
  var pageStart = Math.max(1, parseIntAttr(root, "data-page-start", 1));
  var pageEndAttr = parseIntAttr(root, "data-page-end", 0);
  var currentPage = pageStart;
  var rendering = false;
  var pendingPage = null;
  var pdfDoc = null;
  var pageEnd = pageStart;

  function setStatus(msg) {
    if (status) {
      status.textContent = msg || "";
    }
  }

  function updateChrome() {
    if (pageLabel) {
      pageLabel.textContent = "Page " + currentPage + " of " + pageEnd +
        (pageStart > 1 || pageEndAttr > 0 ? " (lesson " + pageStart + "–" + pageEnd + ")" : "");
    }
    var prev = root.querySelector("[data-pdf-prev]");
    var next = root.querySelector("[data-pdf-next]");
    if (prev) {
      prev.disabled = currentPage <= pageStart;
    }
    if (next) {
      next.disabled = currentPage >= pageEnd;
    }
  }

  function computeFitScale(page) {
    var base = page.getViewport({ scale: 1 });
    var availW = Math.max(120, stage.clientWidth - 8);
    var availH = Math.max(120, stage.clientHeight - 8);
    if (fitMode === "page") {
      return Math.min(availW / base.width, availH / base.height);
    }
    if (fitMode === "width") {
      return availW / base.width;
    }
    return customScale;
  }

  async function renderPage(num) {
    if (!pdfDoc) {
      return;
    }
    if (rendering) {
      pendingPage = num;
      return;
    }
    rendering = true;
    try {
      currentPage = clamp(num, pageStart, pageEnd);
      updateChrome();
      var page = await pdfDoc.getPage(currentPage);
      var scale = computeFitScale(page);
      var viewport = page.getViewport({ scale: scale });
      var outputScale = window.devicePixelRatio || 1;
      canvas.width = Math.floor(viewport.width * outputScale);
      canvas.height = Math.floor(viewport.height * outputScale);
      canvas.style.width = Math.floor(viewport.width) + "px";
      canvas.style.height = Math.floor(viewport.height) + "px";
      var transform = outputScale !== 1 ? [outputScale, 0, 0, outputScale, 0, 0] : null;
      await page.render({
        canvasContext: ctx,
        viewport: viewport,
        transform: transform
      }).promise;
      setStatus("");
    } catch (err) {
      setStatus("Could not render that page.");
      console.error(err);
    } finally {
      rendering = false;
      if (pendingPage != null && pendingPage !== currentPage) {
        var next = pendingPage;
        pendingPage = null;
        renderPage(next);
      } else {
        pendingPage = null;
      }
    }
  }

  root.querySelector("[data-pdf-prev]")?.addEventListener("click", function () {
    renderPage(currentPage - 1);
  });
  root.querySelector("[data-pdf-next]")?.addEventListener("click", function () {
    renderPage(currentPage + 1);
  });
  root.querySelector("[data-pdf-zoom-in]")?.addEventListener("click", function () {
    fitMode = "custom";
    customScale = clamp((customScale || 1) * 1.2, 0.25, 6);
    renderPage(currentPage);
  });
  root.querySelector("[data-pdf-zoom-out]")?.addEventListener("click", function () {
    fitMode = "custom";
    customScale = clamp((customScale || 1) / 1.2, 0.25, 6);
    renderPage(currentPage);
  });
  root.querySelector("[data-pdf-fit-width]")?.addEventListener("click", function () {
    fitMode = "width";
    renderPage(currentPage);
  });
  root.querySelector("[data-pdf-fit-page]")?.addEventListener("click", function () {
    fitMode = "page";
    renderPage(currentPage);
  });
  root.querySelector("[data-pdf-reset]")?.addEventListener("click", function () {
    fitMode = "width";
    customScale = 1;
    renderPage(pageStart);
  });

  var jump = root.querySelector("[data-pdf-jump]");
  if (jump) {
    jump.addEventListener("change", function () {
      var n = parseInt(jump.value, 10);
      if (Number.isFinite(n)) {
        renderPage(n);
      }
    });
  }

  window.addEventListener("resize", function () {
    if (fitMode === "width" || fitMode === "page") {
      renderPage(currentPage);
    }
  });

  setStatus("Loading PDF…");
  try {
    pdfDoc = await pdfjsLib.getDocument({ url: url, withCredentials: true }).promise;
    if (pageEndAttr > 0) {
      pageEnd = Math.min(pageEndAttr, pdfDoc.numPages);
    } else {
      pageEnd = pdfDoc.numPages;
    }
    pageStart = clamp(pageStart, 1, pdfDoc.numPages);
    pageEnd = clamp(pageEnd, pageStart, pdfDoc.numPages);
    currentPage = pageStart;
    if (jump) {
      jump.min = String(pageStart);
      jump.max = String(pageEnd);
      jump.value = String(pageStart);
    }
    await renderPage(currentPage);
  } catch (err) {
    setStatus("Could not open that PDF.");
    console.error(err);
  }
}

document.querySelectorAll("[data-pdf-viewer]").forEach(function (el) {
  mountViewer(el);
});
