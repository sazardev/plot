(() => {
  const root = document.querySelector("[data-pg]");
  if (!root) return;

  const MAX_ROWS = 5000;
  const MAX_FILE_BYTES = 12 * 1024 * 1024;
  const COLORS = ["#c8f560", "#5ad7e6", "#a797ff", "#ffbd6a", "#f18cb4", "#73e2a4", "#8fb4ff", "#e5e88f"];
  const NUMBER = /^[-+]?(\d+\.?\d*|\.\d+)([eE][-+]?\d+)?$/;

  const $ = (selector, scope = root) => scope.querySelector(selector);
  const $$ = (selector, scope = root) => Array.from(scope.querySelectorAll(selector));

  const els = {
    text: $("[data-pg-text]"),
    status: $("[data-pg-status]"),
    title: $("[data-pg-title]"),
    meta: $("[data-pg-meta]"),
    canvas: $("[data-pg-canvas]"),
    empty: $("[data-pg-empty]"),
    chart: $(".pg-chart"),
    preview: $("[data-pg-preview]"),
    file: $("[data-pg-file]"),
    drop: $("[data-pg-drop]"),
    fileName: $("[data-pg-file-name]"),
    sheetWrap: $("[data-pg-sheet-wrap]"),
    sheet: $("[data-pg-sheet]"),
    sql: $("[data-pg-sql]"),
    sqlTable: $("[data-pg-table]"),
    sqlQuery: $("[data-pg-query]"),
    download: $('[data-pg-action="download"]'),
    copyCommand: $('[data-pg-action="copy-command"]'),
    controls: $("[data-pg-controls]"),
    label: $("[data-pg-label]"),
    series: $("[data-pg-series]"),
  };

  const reduceMotion = window.matchMedia("(prefers-reduced-motion: reduce)").matches;
  if (window.Chart) {
    window.Chart.defaults.font.family = "Geist Mono, monospace";
    window.Chart.defaults.font.size = 10;
  }
  let table = null;
  let chart = null;
  let labelIndex = -1;
  let selectedSeries = new Set();
  let workbook = null;
  let workbookName = "";
  let sqlDb = null;
  let sqlSource = "";
  let sqlLoad = null;
  let xlsxLoad = null;

  const setStatus = (message, kind = "") => {
    els.status.textContent = message;
    els.status.classList.toggle("is-ok", kind === "ok");
    els.status.classList.toggle("is-error", kind === "error");
  };

  const setActive = (selector, active) => {
    $$(selector).forEach((button) => {
      const isActive = button === active;
      button.classList.toggle("is-active", isActive);
      if (button.getAttribute("role") === "tab") button.setAttribute("aria-selected", String(isActive));
    });
  };

  const loadScript = (src) =>
    new Promise((resolve, reject) => {
      const existing = document.querySelector(`script[src="${src}"]`);
      if (existing) {
        if (existing.dataset.loaded === "true") return resolve();
        existing.addEventListener("load", () => resolve(), { once: true });
        existing.addEventListener("error", () => reject(new Error(`Could not load ${src}`)), { once: true });
        return;
      }
      const script = document.createElement("script");
      script.src = src;
      script.async = true;
      script.addEventListener("load", () => {
        script.dataset.loaded = "true";
        resolve();
      });
      script.addEventListener("error", () => reject(new Error(`Could not load ${src}`)), { once: true });
      document.head.appendChild(script);
    });

  const ensureXLSX = async () => {
    if (window.XLSX) return window.XLSX;
    if (!xlsxLoad) {
      xlsxLoad = loadScript("assets/vendor/xlsx.full.min.js").then(() => {
        if (!window.XLSX) throw new Error("The Excel parser did not initialize.");
        return window.XLSX;
      });
    }
    return xlsxLoad;
  };

  const ensureSQL = async () => {
    if (window.initSqlJs) return sqlLoad;
    if (!sqlLoad) {
      sqlLoad = loadScript("assets/vendor/sql-wasm.js").then(() =>
        window.initSqlJs({ locateFile: (file) => `assets/vendor/${file}` })
      );
    }
    return sqlLoad;
  };

  const parseNumber = (value) => {
    if (typeof value === "number") return Number.isFinite(value) ? value : null;
    if (typeof value !== "string") return null;
    let text = value.trim();
    if (!text) return null;
    let percent = false;
    if (text.endsWith("%")) {
      percent = true;
      text = text.slice(0, -1).trim();
    }
    text = text.replace(/^[$€£¥]\s?/, "").replace(/,/g, "");
    if (!NUMBER.test(text)) return null;
    let number = Number(text);
    if (!Number.isFinite(number)) return null;
    if (percent) number /= 100;
    return number;
  };

  const cellValue = (value) => {
    if (value === null || value === undefined) return null;
    if (typeof value === "number") return Number.isFinite(value) ? value : null;
    if (typeof value === "boolean") return value ? "true" : "false";
    if (value instanceof Date) return value.toISOString().slice(0, 10);
    if (typeof value === "object") {
      if (value instanceof Uint8Array) return "[blob]";
      try {
        return JSON.stringify(value);
      } catch {
        return String(value);
      }
    }
    const text = String(value).trim();
    if (!text) return null;
    const number = parseNumber(text);
    return number === null ? text : number;
  };

  const displayCell = (value) => {
    if (value === null || value === undefined) return "";
    if (typeof value === "number") return Number.isInteger(value) ? String(value) : String(value);
    return String(value);
  };

  const uniqueNames = (values) => {
    const seen = new Map();
    return values.map((value, index) => {
      let name = String(value ?? "").trim() || `column_${index + 1}`;
      const count = seen.get(name) || 0;
      seen.set(name, count + 1);
      if (count > 0) name = `${name}_${count + 1}`;
      return name;
    });
  };

  const looksLikeHeader = (first, rest) => {
    const cells = first.filter((value) => value !== null && value !== undefined && String(value).trim() !== "");
    if (!cells.length) return false;
    const numbers = cells.filter((value) => parseNumber(value) !== null).length;
    if (numbers === cells.length) return false;
    const restHasNumber = rest.some((row) => row.some((value) => parseNumber(value) !== null));
    return restHasNumber || rest.length === 0;
  };

  const buildTable = (matrix, options = {}) => {
    const cleaned = matrix
      .map((row) => row.map(cellValue))
      .filter((row) => row.some((value) => value !== null));
    if (!cleaned.length) throw new Error("The data is empty.");

    const header = options.header ?? looksLikeHeader(cleaned[0], cleaned.slice(1));
    const rawColumns = header ? cleaned[0] : [];
    const dataRows = header ? cleaned.slice(1) : cleaned;
    if (!dataRows.length) throw new Error("The table has a header but no rows.");

    const width = Math.max(...cleaned.map((row) => row.length));
    const columns = uniqueNames(
      header
        ? Array.from({ length: width }, (_, index) => rawColumns[index] ?? "")
        : Array.from({ length: width }, (_, index) => `column_${index + 1}`)
    );
    const rows = dataRows.slice(0, MAX_ROWS).map((row) =>
      Array.from({ length: width }, (_, index) => (row[index] === undefined ? null : row[index]))
    );

    const numeric = columns.map((_, column) => {
      let present = 0;
      let numbers = 0;
      rows.forEach((row) => {
        if (row[column] === null) return;
        present++;
        if (typeof row[column] === "number") numbers++;
      });
      return numbers > 0 && numbers >= Math.min(2, present) && numbers >= present * 0.6;
    });

    return {
      columns,
      rows,
      numeric,
      title: options.title || "Untitled table",
      source: options.source || "pasted data",
      truncated: dataRows.length > MAX_ROWS,
    };
  };

  const parseDelimited = (text) => {
    const source = text.replace(/^\uFEFF/, "");
    const firstLines = source.split(/\r?\n/).filter((line) => line.trim()).slice(0, 4);
    const counts = { ",": 0, "\t": 0, ";": 0, "|": 0 };
    firstLines.forEach((line) => {
      Object.keys(counts).forEach((delimiter) => {
        let quoted = false;
        for (let index = 0; index < line.length; index++) {
          if (line[index] === '"') {
            if (quoted && line[index + 1] === '"') {
              index++;
            } else {
              quoted = !quoted;
            }
          } else if (!quoted && line[index] === delimiter) {
            counts[delimiter]++;
          }
        }
      });
    });
    let delimiter = ",";
    Object.entries(counts).forEach(([candidate, count]) => {
      if (count > counts[delimiter]) delimiter = candidate;
    });

    const rows = [];
    let row = [];
    let field = "";
    let quoted = false;
    for (let index = 0; index < source.length; index++) {
      const char = source[index];
      if (quoted) {
        if (char === '"') {
          if (source[index + 1] === '"') {
            field += '"';
            index++;
          } else {
            quoted = false;
          }
        } else {
          field += char;
        }
        continue;
      }
      if (char === '"') {
        quoted = true;
      } else if (char === delimiter) {
        row.push(field);
        field = "";
      } else if (char === "\n") {
        row.push(field);
        rows.push(row);
        row = [];
        field = "";
      } else if (char !== "\r") {
        field += char;
      }
    }
    if (field !== "" || row.length) {
      row.push(field);
      rows.push(row);
    }
    while (rows.length && rows[rows.length - 1].every((cell) => String(cell).trim() === "")) rows.pop();
    return { rows, delimiter };
  };

  const splitMarkdownRow = (line) => {
    let text = line.trim();
    if (text.startsWith("|")) text = text.slice(1);
    if (text.endsWith("|")) text = text.slice(0, -1);
    const cells = [];
    let current = "";
    for (let index = 0; index < text.length; index++) {
      const char = text[index];
      if (char === "\\" && text[index + 1] === "|") {
        current += "|";
        index++;
      } else if (char === "|") {
        cells.push(current.trim());
        current = "";
      } else {
        current += char;
      }
    }
    cells.push(current.trim());
    return cells;
  };

  const markdownSeparator = (line) => {
    if (!line.includes("|")) return false;
    const cells = splitMarkdownRow(line);
    return cells.length > 0 && cells.every((cell) => /^:?-{3,}:?$/.test(cell.trim()));
  };

  const parseMarkdownTable = (text) => {
    const lines = text.split(/\r?\n/);
    for (let index = 1; index < lines.length; index++) {
      if (!markdownSeparator(lines[index])) continue;
      if (!lines[index - 1].includes("|")) continue;
      const rows = [splitMarkdownRow(lines[index - 1])];
      for (let next = index + 1; next < lines.length; next++) {
        if (!lines[next].includes("|")) break;
        if (!lines[next].trim()) break;
        rows.push(splitMarkdownRow(lines[next]));
      }
      if (rows.length > 1) return rows;
    }
    return null;
  };

  const parseJSONTable = (text) => {
    const data = JSON.parse(text);
    if (Array.isArray(data)) {
      if (!data.length) throw new Error("The JSON array is empty.");
      if (data.every((item) => Array.isArray(item))) {
        return buildTable(data, { source: "JSON" });
      }
      if (data.every((item) => item && typeof item === "object" && !Array.isArray(item))) {
        const keys = [];
        data.slice(0, 200).forEach((item) => {
          Object.keys(item).forEach((key) => {
            if (!keys.includes(key)) keys.push(key);
          });
        });
        return buildTable([keys, ...data.map((item) => keys.map((key) => item[key]))], {
          header: true,
          source: "JSON",
        });
      }
      return buildTable([["value"], ...data.map((value) => [value])], { header: true, source: "JSON" });
    }
    if (data && typeof data === "object") {
      if (Array.isArray(data.columns) && Array.isArray(data.rows)) {
        return buildTable([data.columns, ...data.rows], { header: true, source: "JSON" });
      }
      const values = Object.values(data);
      if (values.length && values.every((value) => Array.isArray(value))) {
        const length = values[0].length;
        if (values.every((value) => value.length === length)) {
          return buildTable(
            [Object.keys(data), ...Array.from({ length }, (_, index) => values.map((value) => value[index]))],
            { header: true, source: "JSON" }
          );
        }
      }
      return buildTable([Object.keys(data), Object.values(data)], { header: true, source: "JSON" });
    }
    throw new Error("The JSON value is not tabular.");
  };

  const looksLikeSQL = (text) => /^\s*(select|with|create\s+table|insert\s+into|begin\s+transaction)\b/i.test(text);

  const titleFromName = (name) => {
    const base = name.replace(/\.[^.]+$/, "").replace(/[_-]+/g, " ").trim();
    if (!base) return "Uploaded table";
    return base.charAt(0).toUpperCase() + base.slice(1);
  };

  const selectChartColumns = (tableData) => {
    const label = tableData.columns.findIndex((_, index) => !tableData.numeric[index] && tableData.rows.some((row) => row[index] !== null));
    const values = tableData.numeric.map((isNumeric, index) => (isNumeric ? index : -1)).filter((index) => index >= 0);
    return { label, values };
  };

  const selectedValues = () => {
    if (!table) return [];
    return table.numeric
      .map((isNumeric, index) => (isNumeric && selectedSeries.has(index) ? index : -1))
      .filter((index) => index >= 0);
  };

  const colorIndex = (columnIndex) => {
    if (!table) return 0;
    const order = table.numeric.map((isNumeric, index) => (isNumeric ? index : -1)).filter((index) => index >= 0);
    const position = order.indexOf(columnIndex);
    return position >= 0 ? position : 0;
  };

  const renderControls = () => {
    if (!table) {
      els.controls.hidden = true;
      return;
    }
    els.controls.hidden = false;
    els.label.textContent = "";
    const rowOption = document.createElement("option");
    rowOption.value = "-1";
    rowOption.textContent = "Row number";
    els.label.appendChild(rowOption);
    table.columns.forEach((column, index) => {
      if (table.numeric[index]) return;
      if (!table.rows.some((row) => row[index] !== null)) return;
      const option = document.createElement("option");
      option.value = String(index);
      option.textContent = column;
      els.label.appendChild(option);
    });
    els.label.value = String(labelIndex);

    els.series.textContent = "";
    table.numeric.forEach((isNumeric, index) => {
      if (!isNumeric) return;
      const button = document.createElement("button");
      button.type = "button";
      button.className = "pg-chip";
      button.dataset.seriesIndex = String(index);
      button.style.setProperty("--chip-color", COLORS[colorIndex(index) % COLORS.length]);
      button.textContent = table.columns[index];
      button.setAttribute("aria-pressed", String(selectedSeries.has(index)));
      button.classList.toggle("is-active", selectedSeries.has(index));
      els.series.appendChild(button);
    });
  };

  const formatNumber = (value) => {
    if (value === null || value === undefined || Number.isNaN(value)) return "—";
    const absolute = Math.abs(value);
    if (absolute >= 1e9) return `${trimNumber(value / 1e9)}G`;
    if (absolute >= 1e6) return `${trimNumber(value / 1e6)}M`;
    if (absolute >= 1e3) return `${trimNumber(value / 1e3)}k`;
    if (absolute >= 1) return trimNumber(value);
    if (absolute >= 0.001) return trimNumber(value);
    return trimNumber(value);
  };

  const trimNumber = (value) => {
    if (Number.isInteger(value)) return String(value);
    return Number(value.toPrecision(3)).toString();
  };

  const hexAlpha = (hex, alpha) => {
    const clean = hex.replace("#", "");
    const number = parseInt(clean, 16);
    return `rgba(${(number >> 16) & 255}, ${(number >> 8) & 255}, ${number & 255}, ${alpha})`;
  };

  const setDataset = (next, status) => {
    table = next;
    if (chart) {
      chart.destroy();
      chart = null;
    }
    const defaults = selectChartColumns(next);
    labelIndex = defaults.label;
    selectedSeries = new Set(defaults.values);
    els.title.textContent = next.title;
    const truncated = next.truncated ? ` · showing first ${MAX_ROWS.toLocaleString("en")}` : "";
    els.meta.textContent = `${next.rows.length.toLocaleString("en")} rows × ${next.columns.length.toLocaleString("en")} columns · ${next.source}${truncated}`;
    els.download.disabled = false;
    els.copyCommand.disabled = false;
    setStatus(status, "ok");
    renderControls();
    renderPreview();
    drawChart();
  };

  const renderPreview = () => {
    if (!table) {
      els.preview.hidden = true;
      return;
    }
    els.preview.hidden = false;
    els.preview.textContent = "";
    const tableEl = document.createElement("table");
    const head = document.createElement("thead");
    const headRow = document.createElement("tr");
    table.columns.slice(0, 8).forEach((column) => {
      const th = document.createElement("th");
      th.textContent = column;
      headRow.appendChild(th);
    });
    if (table.columns.length > 8) {
      const th = document.createElement("th");
      th.textContent = "…";
      headRow.appendChild(th);
    }
    head.appendChild(headRow);
    tableEl.appendChild(head);

    const body = document.createElement("tbody");
    table.rows.slice(0, 6).forEach((row) => {
      const tr = document.createElement("tr");
      row.slice(0, 8).forEach((value) => {
        const td = document.createElement("td");
        td.textContent = displayCell(value);
        tr.appendChild(td);
      });
      if (row.length > 8) {
        const td = document.createElement("td");
        td.textContent = "…";
        tr.appendChild(td);
      }
      body.appendChild(tr);
    });
    tableEl.appendChild(body);
    els.preview.appendChild(tableEl);
  };

  const chartType = () => $$("[data-pg-chart]").find((button) => button.classList.contains("is-active"))?.dataset.pgChart || "line";

  const showEmpty = (title, message) => {
    els.canvas.hidden = true;
    els.empty.hidden = false;
    els.empty.querySelector("strong").textContent = title;
    els.empty.querySelector("p").textContent = message;
  };

  const drawChart = () => {
    if (!table) return;
    if (!window.Chart) {
      showEmpty("Chart.js is still loading", "Give it a second and try again.");
      return;
    }
    const values = selectedValues();
    if (!values.length) {
      showEmpty("No numeric series", "Select at least one numeric series.");
      return;
    }

    const type = chartType();
    const labels = table.rows.map((row, index) => {
      const value = labelIndex >= 0 ? row[labelIndex] : index + 1;
      return value === null ? `#${index + 1}` : displayCell(value);
    });

    if (type === "bar" || type === "pie" || type === "donut") {
      if (labelIndex < 0) {
        setStatus("No label column detected; using row numbers.", "ok");
      }
    }

    if (type === "bar") {
      const index = values[0];
      if (values.length > 1) {
        setStatus(`Bar uses ${table.columns[index]}; switch to Line to see all numeric series.`, "ok");
      }
      const points = table.rows.slice(0, 80);
      const barLabels = labels.slice(0, 80);
      const data = points.map((row) => (typeof row[index] === "number" ? row[index] : null));
      if (points.length < table.rows.length) {
        setStatus(`Bar preview shows the first ${points.length} rows of ${table.rows.length}.`, "ok");
      }
      makeCartesianChart("bar", barLabels, [
        {
          label: table.columns[index],
          data,
          backgroundColor: COLORS[colorIndex(index) % COLORS.length],
          borderColor: COLORS[colorIndex(index) % COLORS.length],
          borderWidth: 0,
          borderRadius: 4,
          maxBarThickness: 24,
        },
      ]);
      return;
    }

    if (type === "pie" || type === "donut") {
      const index = values[0];
      if (values.length > 1) {
        setStatus(`${type === "pie" ? "Pie" : "Donut"} uses ${table.columns[index]} only.`, "ok");
      }
      const slices = table.rows
        .map((row, rowIndex) => ({
          label: labels[rowIndex],
          value: typeof row[index] === "number" ? row[index] : 0,
        }))
        .filter((slice) => slice.value > 0)
        .sort((a, b) => b.value - a.value);
      if (!slices.length) {
        showEmpty("Nothing to slice", "Pie and donut need positive numeric values.");
        return;
      }
      let visible = slices;
      if (slices.length > 12) {
        const head = slices.slice(0, 11);
        const rest = slices.slice(11).reduce((total, slice) => total + slice.value, 0);
        visible = [...head, { label: "Other", value: rest }];
      }
      makePieChart(type, visible);
      return;
    }

    if (values.length > 8) {
      setStatus(`Line shows the first 8 numeric series of ${values.length}.`, "ok");
    }
    const series = values.slice(0, 8).map((index) => ({
      label: table.columns[index],
      data: table.rows.map((row) => (typeof row[index] === "number" ? row[index] : null)),
      borderColor: COLORS[colorIndex(index) % COLORS.length],
      backgroundColor: hexAlpha(COLORS[colorIndex(index) % COLORS.length], 0.14),
      pointBackgroundColor: COLORS[colorIndex(index) % COLORS.length],
      borderWidth: 2,
      pointRadius: table.rows.length > 80 ? 0 : 3,
      pointHoverRadius: 5,
      tension: 0.28,
      spanGaps: false,
    }));
    makeCartesianChart("line", labels, series);
  };

  const chartOptions = () => ({
    responsive: true,
    maintainAspectRatio: false,
    animation: reduceMotion ? false : { duration: 420 },
    interaction: { mode: "index", intersect: false },
    plugins: {
      legend: {
        display: false,
      },
      tooltip: {
        backgroundColor: "#0b0d10",
        titleColor: "#f2f4e9",
        bodyColor: "#c8ccbe",
        borderColor: "rgba(255,255,255,.14)",
        borderWidth: 1,
        padding: 10,
        displayColors: true,
        callbacks: {
          label: (context) => `${context.dataset.label || context.label}: ${formatNumber(context.parsed.y ?? context.parsed)}`,
        },
      },
    },
  });

  const makeCartesianChart = (type, labels, datasets) => {
    els.empty.hidden = true;
    els.canvas.hidden = false;
    if (chart) chart.destroy();
    const options = chartOptions();
    const config = { type, data: { labels, datasets }, options };
    options.plugins.legend.display = datasets.length > 1;
    options.plugins.legend.position = "bottom";
    options.plugins.legend.labels = {
      color: "#aeb2a3",
      boxWidth: 8,
      boxHeight: 8,
      usePointStyle: true,
      pointStyle: "circle",
      font: { family: "Geist Mono, monospace", size: 10 },
    };
    options.scales = {
      x: {
        grid: { color: "rgba(255,255,255,.05)", drawTicks: false },
        border: { color: "rgba(255,255,255,.1)" },
        ticks: {
          color: "#8d9284",
          maxRotation: 0,
          autoSkip: true,
          maxTicksLimit: 12,
          font: { family: "Geist Mono, monospace", size: 10 },
        },
      },
      y: {
        beginAtZero: type === "bar",
        grid: { color: "rgba(255,255,255,.07)", drawTicks: false },
        border: { display: false },
        ticks: {
          color: "#8d9284",
          maxTicksLimit: 6,
          font: { family: "Geist Mono, monospace", size: 10 },
          callback: (value) => formatNumber(Number(value)),
        },
      },
    };
    chart = new window.Chart(els.canvas.getContext("2d"), config);
    els.canvas.setAttribute("aria-label", `${type} chart with ${datasets.length} series and ${labels.length} labels`);
  };

  const makePieChart = (type, slices) => {
    els.empty.hidden = true;
    els.canvas.hidden = false;
    if (chart) chart.destroy();
    const total = slices.reduce((sum, slice) => sum + slice.value, 0);
    const options = chartOptions();
    const config = {
      type: type === "donut" ? "doughnut" : "pie",
      data: {
        labels: slices.map((slice) => slice.label),
        datasets: [
          {
            data: slices.map((slice) => slice.value),
            backgroundColor: slices.map((_, index) => COLORS[index % COLORS.length]),
            borderColor: "#171824",
            borderWidth: 2,
            hoverOffset: 4,
          },
        ],
      },
      options,
    };
    options.cutout = type === "donut" ? "62%" : "0%";
    options.plugins.legend.display = true;
    options.plugins.legend.position = "bottom";
    options.plugins.legend.labels = {
      color: "#aeb2a3",
      boxWidth: 8,
      boxHeight: 8,
      usePointStyle: true,
      pointStyle: "circle",
      font: { family: "Geist Mono, monospace", size: 10 },
    };
    options.plugins.tooltip.callbacks.label = (context) => {
      const value = Number(context.raw) || 0;
      const percent = total ? ((value / total) * 100).toFixed(1) : "0.0";
      return `${context.label}: ${formatNumber(value)} (${percent}%)`;
    };
    chart = new window.Chart(els.canvas.getContext("2d"), config);
    els.canvas.setAttribute("aria-label", `${type} chart with ${slices.length} slices`);
  };

  const parseText = (text, options = {}) => {
    const trimmed = text.trim();
    if (!trimmed) throw new Error("Paste a table or drop a file first.");
    if (options.hint === "json" || trimmed.startsWith("{") || trimmed.startsWith("[")) {
      try {
        return parseJSONTable(trimmed);
      } catch (error) {
        if (options.hint === "json") throw error;
      }
    }
    const markdown = parseMarkdownTable(trimmed);
    if (markdown) {
      return buildTable(markdown, {
        header: true,
        title: options.title || "Pasted table",
        source: options.source || "Markdown table",
      });
    }
    if (options.hint !== "csv" && looksLikeSQL(trimmed)) {
      throw new Error("This looks like SQL. Upload a .sqlite file or use the SQL panel for a SQLite dump.");
    }
    const parsed = parseDelimited(trimmed);
    return buildTable(parsed.rows, {
      title: options.title || "Pasted table",
      source: options.source || `${parsed.delimiter === "\t" ? "TSV" : parsed.delimiter === ";" ? "semicolon-delimited" : "CSV"} data`,
    });
  };

  const renderText = async () => {
    const text = els.text.value;
    resetWorkbook();
    if (looksLikeSQL(text)) {
      await openSQLDump(text, "pasted SQL");
      return;
    }
    try {
      const next = parseText(text, { source: "pasted table" });
      resetSQL();
      setDataset(next, `Parsed ${next.rows.length} rows × ${next.columns.length} columns.`);
    } catch (error) {
      setStatus(error.message, "error");
    }
  };

  const resetSQL = () => {
    if (sqlDb) {
      sqlDb.close();
      sqlDb = null;
    }
    sqlSource = "";
    els.sql.hidden = true;
    els.sqlTable.textContent = "";
  };

  const resetWorkbook = () => {
    workbook = null;
    els.sheetWrap.hidden = true;
  };

  const listSQLTables = () => {
    if (!sqlDb) return [];
    const result = sqlDb.exec("SELECT name FROM sqlite_master WHERE type = 'table' AND name NOT LIKE 'sqlite_%' ORDER BY name");
    return result.length ? result[0].values.map((row) => String(row[0])) : [];
  };

  const sqlResultTable = (result, title, source) => {
    if (!result.length) throw new Error("The query returned no result set.");
    const { columns, values } = result[0];
    if (!columns.length || !values.length) throw new Error("The query returned no rows.");
    return buildTable([columns, ...values], { header: true, title, source });
  };

  const runSQL = (query, title, source) => {
    if (!sqlDb) {
      setStatus("Upload a SQLite database or a SQL dump first.", "error");
      return;
    }
    try {
      const result = sqlDb.exec(query);
      const next = sqlResultTable(result, title || "Query result", source || "SQLite");
      setDataset(next, `Query returned ${next.rows.length} rows × ${next.columns.length} columns.`);
    } catch (error) {
      setStatus(`SQL error: ${error.message}`, "error");
    }
  };

  const loadSQLTable = (name) => {
    if (!name) return;
    const escaped = name.replace(/"/g, '""');
    els.sqlQuery.value = `SELECT * FROM "${escaped}" LIMIT 500;`;
    runSQL(els.sqlQuery.value, titleFromName(name), `${sqlSource} · ${name}`);
  };

  const showSQLPanel = (tables) => {
    els.sql.hidden = false;
    els.sqlTable.textContent = "";
    tables.forEach((name) => {
      const option = document.createElement("option");
      option.value = name;
      option.textContent = name;
      els.sqlTable.appendChild(option);
    });
    if (tables.length) {
      els.sqlTable.value = tables[0];
      loadSQLTable(tables[0]);
    }
  };

  const openSQLite = async (buffer, name) => {
    setStatus("Opening SQLite database…");
    try {
      const SQL = await ensureSQL();
      resetSQL();
      sqlDb = new SQL.Database(new Uint8Array(buffer));
      sqlSource = name;
      const tables = listSQLTables();
      if (!tables.length) throw new Error("This database has no user tables.");
      showSQLPanel(tables);
      setStatus(`SQLite database ready · ${tables.length} table${tables.length === 1 ? "" : "s"}.`, "ok");
    } catch (error) {
      setStatus(`Could not open SQLite: ${error.message}`, "error");
    }
  };

  const openSQLDump = async (text, name) => {
    if (/^\s*select\b/i.test(text) || /^\s*with\b/i.test(text)) {
      setStatus("A query needs a database. Upload a .sqlite file or paste its result instead.", "error");
      return;
    }
    setStatus("Running SQL locally…");
    try {
      const SQL = await ensureSQL();
      resetSQL();
      sqlDb = new SQL.Database();
      sqlDb.run(text);
      sqlSource = name;
      const tables = listSQLTables();
      if (!tables.length) throw new Error("No tables were created. Use SQLite-compatible CREATE/INSERT statements, or upload a .sqlite file.");
      showSQLPanel(tables);
      setStatus(`SQL loaded · ${tables.length} table${tables.length === 1 ? "" : "s"}.`, "ok");
    } catch (error) {
      setStatus(`Could not run SQL: ${error.message}`, "error");
      resetSQL();
    }
  };

  const loadSheet = (sheetName) => {
    if (!workbook) return;
    try {
      const sheet = workbook.Sheets[sheetName];
      const matrix = window.XLSX.utils.sheet_to_json(sheet, { header: 1, raw: false, defval: "" });
      const next = buildTable(matrix, {
        title: `${titleFromName(workbookName)} · ${sheetName}`,
        source: "Excel",
      });
      setDataset(next, `Loaded ${sheetName} · ${next.rows.length} rows × ${next.columns.length} columns.`);
    } catch (error) {
      setStatus(`Could not read sheet: ${error.message}`, "error");
    }
  };

  const openWorkbook = async (buffer, name) => {
    setStatus("Reading workbook…");
    try {
      const XLSX = await ensureXLSX();
      workbook = XLSX.read(new Uint8Array(buffer), { type: "array" });
      workbookName = name;
      const sheets = workbook.SheetNames.filter((sheet) => workbook.Sheets[sheet]);
      if (!sheets.length) throw new Error("The workbook has no sheets.");
      els.sheet.textContent = "";
      sheets.forEach((sheet) => {
        const option = document.createElement("option");
        option.value = sheet;
        option.textContent = sheet;
        els.sheet.appendChild(option);
      });
      els.sheetWrap.hidden = sheets.length < 2;
      els.sheet.value = sheets[0];
      loadSheet(sheets[0]);
    } catch (error) {
      setStatus(`Could not read workbook: ${error.message}`, "error");
    }
  };

  const handleFile = async (file) => {
    if (!file) return;
    if (file.size > MAX_FILE_BYTES) {
      setStatus(`File is larger than ${MAX_FILE_BYTES / (1024 * 1024)} MB. Try a smaller export.`, "error");
      return;
    }
    els.fileName.textContent = `${file.name} · ${(file.size / 1024).toFixed(1)} KB`;
    const extension = (file.name.split(".").pop() || "").toLowerCase();
    try {
      if (["xlsx", "xls"].includes(extension)) {
        resetSQL();
        await openWorkbook(await file.arrayBuffer(), file.name);
        return;
      }
      if (["db", "sqlite", "sqlite3"].includes(extension)) {
        resetSQL();
        workbook = null;
        els.sheetWrap.hidden = true;
        await openSQLite(await file.arrayBuffer(), file.name);
        return;
      }
      const text = await file.text();
      if (extension === "sql") {
        workbook = null;
        els.sheetWrap.hidden = true;
        await openSQLDump(text, file.name);
        return;
      }
      resetSQL();
      workbook = null;
      els.sheetWrap.hidden = true;
      const hint = extension === "json" ? "json" : extension === "md" || extension === "markdown" ? "csv" : "csv";
      const next = parseText(text, {
        hint,
        title: titleFromName(file.name),
        source: extension === "json" ? "JSON file" : extension === "md" || extension === "markdown" ? "Markdown file" : "CSV file",
      });
      if (extension === "md" || extension === "markdown") {
        next.source = "Markdown file";
      }
      setDataset(next, `Loaded ${file.name} · ${next.rows.length} rows × ${next.columns.length} columns.`);
    } catch (error) {
      setStatus(`Could not read ${file.name}: ${error.message}`, "error");
    }
  };

  const csvValue = (value) => {
    if (value === null || value === undefined) return "";
    const text = String(value);
    return /[",\n\r]/.test(text) ? `"${text.replace(/"/g, '""')}"` : text;
  };

  const downloadCSV = () => {
    if (!table) return;
    const lines = [table.columns.map(csvValue).join(",")];
    table.rows.forEach((row) => lines.push(row.map(csvValue).join(",")));
    const blob = new Blob([lines.join("\n")], { type: "text/csv;charset=utf-8" });
    const url = URL.createObjectURL(blob);
    const link = document.createElement("a");
    link.href = url;
    link.download = "pl-playground.csv";
    document.body.appendChild(link);
    link.click();
    link.remove();
    URL.revokeObjectURL(url);
    setStatus("CSV downloaded. Run `pl pl-playground.csv` to render it in your terminal.", "ok");
  };

  const copyCommand = async () => {
    const command = `pl pl-playground.csv --type ${chartType()}`;
    try {
      await navigator.clipboard.writeText(command);
      const button = els.copyCommand;
      const previous = button.textContent;
      button.textContent = "Copied command";
      setTimeout(() => {
        button.textContent = previous;
      }, 1400);
      setStatus(`Copied: ${command}`, "ok");
    } catch {
      setStatus(`Copy this command: ${command}`, "ok");
    }
  };

  const loadSample = () => {
    els.text.value = els.text.defaultValue;
    switchPane("paste");
    resetSQL();
    resetWorkbook();
    try {
      const next = parseText(els.text.value, { title: "Revenue by month", source: "Markdown table" });
      setDataset(next, `Sample loaded · ${next.rows.length} rows × ${next.columns.length} columns.`);
    } catch (error) {
      setStatus(error.message, "error");
    }
  };

  const switchPane = (name) => {
    const button = $(`[data-pg-tab="${name}"]`);
    if (!button) return;
    setActive("[data-pg-tab]", button);
    $$("[data-pg-pane]").forEach((pane) => {
      pane.hidden = pane.dataset.pgPane !== name;
    });
    const render = $('[data-pg-action="render"]');
    if (render) render.hidden = name !== "paste";
    const sample = $('[data-pg-action="sample"]');
    if (sample) sample.hidden = false;
  };

  $$("[data-pg-tab]").forEach((button) => {
    button.addEventListener("click", () => switchPane(button.dataset.pgTab));
  });

  $$("[data-pg-chart]").forEach((button) => {
    button.addEventListener("click", () => {
      setActive("[data-pg-chart]", button);
      drawChart();
    });
  });

  els.label.addEventListener("change", () => {
    labelIndex = Number(els.label.value);
    drawChart();
  });

  els.series.addEventListener("click", (event) => {
    const button = event.target.closest("[data-series-index]");
    if (!button) return;
    const index = Number(button.dataset.seriesIndex);
    if (selectedSeries.has(index)) {
      if (selectedSeries.size === 1) {
        setStatus("Keep at least one series selected.", "error");
        return;
      }
      selectedSeries.delete(index);
    } else {
      selectedSeries.add(index);
    }
    button.classList.toggle("is-active", selectedSeries.has(index));
    button.setAttribute("aria-pressed", String(selectedSeries.has(index)));
    drawChart();
  });

  $$("[data-pg-action]").forEach((button) => {
    button.addEventListener("click", async () => {
      const action = button.dataset.pgAction;
      if (action === "clipboard") {
        try {
          const text = await navigator.clipboard.readText();
          if (!text.trim()) throw new Error("The clipboard is empty.");
          els.text.value = text;
          switchPane("paste");
          renderText();
        } catch (error) {
          setStatus(`Clipboard unavailable: ${error.message}`, "error");
        }
      }
      if (action === "sample") loadSample();
      if (action === "render") renderText();
      if (action === "download") downloadCSV();
      if (action === "copy-command") copyCommand();
      if (action === "run-sql") runSQL(els.sqlQuery.value, els.sqlTable.value || "Query result", sqlSource);
    });
  });

  els.file.addEventListener("change", () => {
    handleFile(els.file.files && els.file.files[0]);
  });

  ["dragenter", "dragover"].forEach((event) => {
    els.drop.addEventListener(event, (e) => {
      e.preventDefault();
      els.drop.classList.add("is-over");
    });
  });

  ["dragleave", "drop"].forEach((event) => {
    els.drop.addEventListener(event, (e) => {
      e.preventDefault();
      els.drop.classList.remove("is-over");
    });
  });

  els.drop.addEventListener("drop", (e) => {
    const file = e.dataTransfer?.files?.[0];
    if (file) handleFile(file);
  });

  els.sheet.addEventListener("change", () => loadSheet(els.sheet.value));
  els.sqlTable.addEventListener("change", () => loadSQLTable(els.sqlTable.value));
  els.sqlQuery.addEventListener("keydown", (event) => {
    if ((event.metaKey || event.ctrlKey) && event.key === "Enter") {
      event.preventDefault();
      runSQL(els.sqlQuery.value, els.sqlTable.value || "Query result", sqlSource);
    }
  });

  document.addEventListener("paste", (event) => {
    if (event.target === els.text) return;
    const file = Array.from(event.clipboardData?.files || [])[0];
    if (file) {
      event.preventDefault();
      switchPane("file");
      handleFile(file);
    }
  });

  els.text.addEventListener("keydown", (event) => {
    if ((event.metaKey || event.ctrlKey) && event.key === "Enter") {
      event.preventDefault();
      renderText();
    }
  });

  if (document.fonts && document.fonts.ready) {
    document.fonts.ready.then(() => {
      if (table) drawChart();
    });
  }

  loadSample();
})();
