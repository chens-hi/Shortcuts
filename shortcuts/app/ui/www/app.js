    initLocalizedHtml();



    const DEPLOY_LABELS = T.dynamic.status;


    const RUNTIME_LABELS = T.dynamic.runtime;

    const MAX_DISPLAY_NAME = 12;

    const OPERATION_TIMEOUT_MS = 300000;

    const API_BASE = '/app/shortcuts';

    const APPNAME_PATTERN = /^[a-z][a-z0-9-]{1,11}$/;

    const DEFAULT_ICON = '/app/shortcuts/icons/child_entry_256.png';



    const form = document.querySelector("#entryForm");

    const overlay = document.querySelector("#entryFormOverlay");

    const entryModal = document.querySelector(".entry-modal");

    const entryFormTitle = document.querySelector("#entryFormTitle");

    const entryFormCloseButton = document.querySelector("#entryFormCloseButton");

    const operationOverlay = document.querySelector("#operationOverlay");

    const operationMessage = document.querySelector("#operationMessage");

    const formAppnameInput = document.querySelector("#formAppname");

    const displayNameInput = document.querySelector("#displayName");

    const displayIconInput = document.querySelector("#displayIcon");

    const iconPreview = document.querySelector("#iconPreview");

    const portInput = document.querySelector("#port");

    const protocolInput = document.querySelector("#protocol");

    const pathInput = document.querySelector("#path");

    const openModeInput = document.querySelector("#openMode");

    const typePickerOverlay = document.querySelector("#typePickerOverlay");

    const typePickerGrid = document.querySelector("#typePickerGrid");

    const typePickerCloseButton = document.querySelector("#typePickerCloseButton");

    const urlInput = document.querySelector("#url");

    const urlField = document.getElementById("urlField");
;

    const descriptionInput = document.querySelector("#description");

    const addEntryButton = document.querySelector("#addEntryButton");

    const cancelButton = document.querySelector("#cancelButton");

    const themeButton = document.querySelector("#themeButton");

    const entryRows = document.querySelector("#entryRows");

    const entryCount = document.querySelector("#entryCount");

    const listNotice = document.querySelector("#listNotice");

    const listWrap = document.querySelector("#listWrap");

    const toast = document.querySelector("#toast");

    const orderAnnounce = document.querySelector("#orderAnnounce");

    const cropOverlay = document.querySelector("#cropOverlay");

    const cropCanvas = document.querySelector("#cropCanvas");

    const cropScale = document.querySelector("#cropScale");

    const cropResetButton = document.querySelector("#cropResetButton");

    const cropApplyButton = document.querySelector("#cropApplyButton");

    const cropCloseButton = document.querySelector("#cropCloseButton");

    const iconCropButton = document.querySelector("#iconCropButton");

    const iconRestoreButton = document.querySelector("#iconRestoreButton");

    const iconClearButton = document.querySelector("#iconClearButton");


    const listLoading = document.querySelector("#listLoading");


    let toastTimer = null;

    let iconPreviewUrl = "";

    let pendingIcon = "keep";

    let cropState = { image: null, scale: 1, offsetX: 0, offsetY: 0 };

    let isDraggingCrop = false;

    let dragStart = { x: 0, y: 0, offsetX: 0, offsetY: 0 };

    let refreshEntriesPromise = null;

    let refreshTimer = null;

    let currentEntries = [];

    // 行拖拽状态。不要复用 dragStart：那是图标裁剪的指针起点。
    // isReordering 覆盖整个拖拽过程，isSavingOrder 覆盖整个保存链，
    // 两者都会挡住 10s 定时刷新和在途请求的重绘。
    let rowDragState = null;

    let isReordering = false;

    let isSavingOrder = false;

    let orderSaveTimer = null;

    let orderSaveChain = Promise.resolve();

    let orderSavePending = 0;

    let currentEditAppname = null;

    let modalOpener = null;

    // 当前表单弹窗锁定的类型（创建时由类型选择弹窗决定，编辑时取自条目）。
    // 类型值必须与 internal/model/entry.go 的 EntryType 常量一致。
    let formType = 'internal_port';

    const FORM_TYPE_LABELS = {
      internal_port: T.html.typeInternalPort,
      external_url: T.html.typeExternalUrl,
    };



    function applyTheme(theme) {

      document.body.dataset.theme = theme === "dark" ? "dark" : "light";

    }

    function getStoredTheme() {

      try {

        return localStorage.getItem("shortcuts-theme");

      } catch (error) {

        return null;

      }

    }

    function setStoredTheme(theme) {

      try {

        localStorage.setItem("shortcuts-theme", theme);

      } catch (error) {

        // fnOS embedded containers may disable local storage; keep theme state window-local.

      }

    }

    function initTheme() {
      // 启动时从 localStorage 恢复明暗主题。之前这个函数定义了却从未被调用，
      // 所以重开页面总是回到浅色；调用点见文件末尾的启动区块。
      applyTheme(getStoredTheme() === "dark" ? "dark" : "light");
    }

    function toggleTheme() {

      const next = document.body.dataset.theme === "dark" ? "light" : "dark";

      applyTheme(next);

      setStoredTheme(next);

    }

    function setOperationButtonsEnabled(enabled) {

      document.querySelectorAll(".entry-actions button").forEach((button) => {

        button.disabled = !enabled;

      });

    }

    function showOperationOverlay(message) {

      operationMessage.textContent = message || T.dynamic.operation.processing;

      setOperationButtonsEnabled(false);

      operationOverlay.classList.add("open");

      operationOverlay.setAttribute("aria-hidden", "false");

    }

    function hideOperationOverlay() {

      setOperationButtonsEnabled(true);

      operationOverlay.classList.remove("open");

      operationOverlay.setAttribute("aria-hidden", "true");

    }

    function gatewayFallbackResponseText(text) {

      return String(text || "");

    }

    function shouldUseGatewayFallback(response, responseText) {

      if (!response) return true;

      const text = gatewayFallbackResponseText(responseText).trim();

      if (response.status === 400 || response.status >= 500) return true;

      if (!text || response.headers?.get?.("content-type")?.includes("application/json") === false) return true;

      try {

        const data = JSON.parse(text);

        if (!data || typeof data !== "object") return true;

      } catch (_) {

        return true;

      }

      return text.includes("EOF") || text.includes("empty JSON request body") || text.includes("empty body");

    }



    async function fetchJsonWithTimeout(url, options = {}) {

      const controller = new AbortController();

      const timeoutMs = options.timeoutMs || OPERATION_TIMEOUT_MS;

      const timer = setTimeout(() => controller.abort(), timeoutMs);

      try {

        return await fetch(url, {

          ...options,

          timeoutMs: undefined,

          signal: controller.signal,

          cache: "no-store",

        });

      } catch (error) {

        if (controller.signal.aborted) {

          throw new Error(ERROR_MESSAGES[ERROR_CODES.TIMEOUT]);

        }

        throw error;

      } finally {

        clearTimeout(timer);

      }

    }

    // 只压行内多余空白（空格/制表符），保留换行符。
    // 上游不能再把换行抹成空格：含换行的消息要在 showToast 里走 expanded
    // 分支按行渲染，否则整段命令输出会变成一行被空格糊住的乱码。
    function oneLine(message) {

      return String(message || "").replace(/[^\S\r\n]+/g, " ").trim();

    }



    function showGeneralToast(message) {

      showToast(oneLine(message));

    }



    function showNotice(message, kind) {

      showToast(oneLine(message), kind);

    }



    // action 由调用方传入（"start" / "stop"），不要从响应 code 反推：
    // 后端 SetRuntime 返回的 Code 是 "start_done" / "stop_done"，永远不等于
    // "enabled"，原先按 code 判断使启用也落进「已停用」分支；而响应里也没有
    // displayName/appname 字段（真名在 entry.displayName），name 只能退回动作
    // 标签，于是两条提示分别变成「已停用：启用」和「已停用：停用」。
    function showRuntimeSuccess(action, name) {

      const template = action === "start" ? T.dynamic.messages.runtimeStarted : T.dynamic.messages.runtimeStopped;

      const label = name || (action === "start" ? T.dynamic.action.enable : T.dynamic.action.disable);

      showToast(formatText(template, { name: label }), "ok");

    }



    function showToast(message, kind) {

      const text = String(message || "");

      const isLong = text.includes("\n") || text.length > 120;

      toast.textContent = isLong ? text : text.replace(/\s*[\r\n]+\s*/g, " ").trim();

      toast.classList.remove("show", "ok", "warn", "expanded");

      if (kind === "ok") toast.classList.add("ok");

      if (kind === "warn" || isLong) toast.classList.add("warn");

      if (isLong) toast.classList.add("expanded");

      toast.classList.add("show");

      clearTimeout(toastTimer);

      toastTimer = setTimeout(() => toast.classList.remove("show"), 5000);

    }

    function saveFailureDetail(response, data) {

      const contentType = response.headers && response.headers.get ? response.headers.get("content-type") : "";

      const detail = (data && data.detail) || (data && data.error) || "";

      return `status=${response.status}${contentType ? ` type=${contentType}` : ""} ${detail}`.trim();

    }

    function entryQueryValues(payload) {

      const query = new URLSearchParams();

      query.set("appname", payload.appname || "");

      query.set("displayName", payload.displayName || "");

      query.set("description", payload.description || "");

      query.set("type", payload.type || "");

      query.set("url", payload.url || "");

      query.set("config", JSON.stringify(payload.config || {}));

      return query;

    }

    function buildSaveRequestUrl(payload) {

      const query = entryQueryValues(payload);

      const queryString = query.toString();

      return queryString ? `${API_BASE}/api/entries?${queryString}` : `${API_BASE}/api/entries`;

    }

    // 主句优先用调用方给定的更具体文案，没有再退回按错误码查的通用文案。
    // 反过来会让「刷新失败」被 SC-1001 的「读取失败」盖掉、「启用失败」被
    // SC-3002 的「操作失败」盖掉，几处不同场景的提示挤成同一句话。
    // 第一个参数原来是个从没被引用的 message，调用方把真实错误文本传进去后
    // 被静默丢弃，用户只看到「部署失败，请重试」，现已删掉这个死参数。
    function userMessage(code, fallback, detail, detailFallback) {

      const resolved = code || ERROR_CODES.UNKNOWN;

      const base = fallback || ERROR_MESSAGES[resolved] || ERROR_MESSAGES[ERROR_CODES.UNKNOWN];

      // 错误码只挂在主句后面，诊断详情单独成段。
      // 原来把 detail 塞进主句再套错误码，多行输出会让错误码飘到最后一行
      // 末尾（「命令输出 （错误码：SC-4002）」），整段读不通。
      const head = formatText(T.dynamic.messages.errorCodeSuffix, { message: base, code: resolved });

      const detailText = detail ? String(detailFallback || detail).trim() : "";

      return detailText ? `${head}\n\n${detailText}` : head;

    }



    // 异常分支的 error.message 才是真实原因，但超时文案与主句完全重复，
    // 不必在详情段里再贴一遍。
    function errorDetail(error) {

      const text = String((error && error.message) || "").trim();

      if (!text || text === ERROR_MESSAGES[ERROR_CODES.TIMEOUT]) return "";

      return text;

    }

    function localMessage(code, fallback) {

      return SUCCESS_MESSAGES[code] || ERROR_MESSAGES[code] || fallback || ERROR_MESSAGES[ERROR_CODES.UNKNOWN];

    }

    function entryDisplayName(appname, fallback) {

      return currentEntries.find((item) => item.appname === appname)?.displayName || fallback || "";

    }

    function escapeHtml(value) {

      return String(value == null ? "" : value)

        .replace(/&/g, "&amp;")

        .replace(/</g, "&lt;")

        .replace(/>/g, "&gt;")

        .replace(/"/g, "&quot;")

        .replace(/'/g, "&#39;");

    }

    function revokeIconPreview() {

      cropState = { image: null, scale: 1, offsetX: 0, offsetY: 0 };

      if (iconPreviewUrl) {

        URL.revokeObjectURL(iconPreviewUrl);

        iconPreviewUrl = "";

      }

    }

    function setDescriptionValue(value) {

      descriptionInput.textContent = value || "";

    }

    function getDescriptionValue() {

      return descriptionInput.textContent.replace(/\u00a0/g, " ").replace(/^\s+/, "").replace(/\s+$/, "");

    }

    function clearFileInput() {

      displayIconInput.value = '';

    }

    function buildIconPreviewHtml(imageAlt, imageSrc) {

      return '<img alt="' + escapeHtml(imageAlt) + '" src="' + escapeHtml(imageSrc) + '"><span class="icon-preview-overlay">' + escapeHtml(T.html.iconChoose) + '</span>';

    }

    function updateIconButtonVisibility() {

      const entry = currentEditAppname ? currentEntries.find((item) => item.appname === currentEditAppname) : null;

      const hasSavedIcon = !!(entry && entry.displayIcon);

      if (pendingIcon === "keep") {

        // 无待定变更：编辑已有自定义图标时仅显示清除按钮

        iconClearButton.classList.toggle("hidden", !hasSavedIcon);

        iconRestoreButton.classList.add("hidden");

        iconCropButton.classList.add("hidden");

      } else if (pendingIcon === "default") {

        // 已清除图标待保存：仅显示还原按钮

        iconClearButton.classList.add("hidden");

        iconRestoreButton.classList.remove("hidden");

        iconCropButton.classList.add("hidden");

      } else {

        // 本地待上传预览：显示还原 + 裁剪按钮

        iconClearButton.classList.add("hidden");

        iconRestoreButton.classList.remove("hidden");

        iconCropButton.classList.remove("hidden");

      }

    }

    function showDefaultIconPreview() {

      iconPreview.innerHTML = buildIconPreviewHtml(T.html.defaultIconAlt, DEFAULT_ICON);

    }

    function persistedIconSrc(appname) {

      return API_BASE + "/icon/" + encodeURIComponent(appname) + "?size=256&t=" + Date.now();

    }

    function renderPersistedIcon(appname) {

      revokeIconPreview();

      if (appname) {

        iconPreview.innerHTML = buildIconPreviewHtml(T.dynamic.iconPreviewAlt, persistedIconSrc(appname));

      } else {

        showDefaultIconPreview();

      }

      updateIconButtonVisibility();

    }

    function renderPendingIconPreview() {

      revokeIconPreview();

      if (pendingIcon && typeof pendingIcon === "object") {

        if (pendingIcon.dataUrl) {

          iconPreview.innerHTML = buildIconPreviewHtml(T.dynamic.iconPreviewAlt, pendingIcon.dataUrl);

        } else if (pendingIcon.file) {

          iconPreviewUrl = URL.createObjectURL(pendingIcon.file);

          iconPreview.innerHTML = buildIconPreviewHtml(T.dynamic.iconPreviewAlt, iconPreviewUrl);

        }

      }

      updateIconButtonVisibility();

    }



    function renderIconPreview(entry) {

      pendingIcon = "keep";

      renderPersistedIcon(entry && entry.appname ? entry.appname : "");

    }

    function resetCropState() {

      cropState = { image: null, scale: 1, offsetX: 0, offsetY: 0 };

    }

    function fitCropScale(image) {

      const width = image.naturalWidth || image.width;

      const height = image.naturalHeight || image.height;

      return Math.max(0.001, 256 / Math.max(width, height));

    }



    function defaultCropScale(image) {

      return fitCropScale(image);

    }



    function maxCropScale(image) {

      return defaultCropScale(image) * 2;

    }



    function minCropScale(image) {

      return fitCropScale(image);

    }

    function clampCropOffset(value, min, max) {

      return Math.min(max, Math.max(min, value));

    }

    function normalizeCropState() {

      if (!cropState.image) return;

      const drawWidth = (cropState.image.naturalWidth || cropState.image.width) * cropState.scale;

      const drawHeight = (cropState.image.naturalHeight || cropState.image.height) * cropState.scale;

      cropState.offsetX = clampCropOffset(cropState.offsetX, Math.min(256 - drawWidth, 0), Math.max(256 - drawWidth, 0));

      cropState.offsetY = clampCropOffset(cropState.offsetY, Math.min(256 - drawHeight, 0), Math.max(256 - drawHeight, 0));

    }

    function drawCropCanvas() {

      const context = cropCanvas.getContext("2d");

      context.clearRect(0, 0, 256, 256);

      if (!cropState.image) return;

      normalizeCropState();

      context.drawImage(cropState.image, cropState.offsetX, cropState.offsetY, cropState.image.naturalWidth * cropState.scale, cropState.image.naturalHeight * cropState.scale);

      cropScale.value = String(Math.round((cropState.scale / defaultCropScale(cropState.image) - 1) * 100));

    }

    function restoreSavedIcon() {

      clearFileInput();

      pendingIcon = "keep";

      renderPersistedIcon(currentEditAppname);

    }

    function resetIconToDefault() {

      clearFileInput();

      pendingIcon = "default";

      revokeIconPreview();

      showDefaultIconPreview();

      updateIconButtonVisibility();

    }

    function applyCrop() {

      const file = displayIconInput.files && displayIconInput.files[0];

      pendingIcon = { file: file || null, dataUrl: cropCanvas.toDataURL("image/png") };

      cropOverlay.classList.remove("open");

      renderPendingIconPreview();

    }

    function resetCrop() {

      if (!cropState.image) return;

      cropState.scale = defaultCropScale(cropState.image);

      cropState.offsetX = (256 - cropState.image.naturalWidth * cropState.scale) / 2;

      cropState.offsetY = (256 - cropState.image.naturalHeight * cropState.scale) / 2;

      drawCropCanvas();

    }

    function setCropScale(percent) {

      if (!cropState.image) return;

      const baseScale = defaultCropScale(cropState.image);

      const minScale = minCropScale(cropState.image);

      const maxScale = maxCropScale(cropState.image);

      cropState.scale = Math.max(minScale, Math.min(maxScale, baseScale * (1 + percent / 100)));

      cropScale.value = String(Math.round((cropState.scale / baseScale - 1) * 100));

      const previousDrawWidth = cropState.image.naturalWidth * cropState.scale;

      const previousDrawHeight = cropState.image.naturalHeight * cropState.scale;

      cropState.offsetX = (256 - previousDrawWidth) / 2;

      cropState.offsetY = (256 - previousDrawHeight) / 2;

      drawCropCanvas();

    }

    async function openCropModal() {

      if (!pendingIcon || typeof pendingIcon !== "object") return;

      const cropSource = pendingIcon.file || pendingIcon.dataUrl;

      if (!cropSource) return;

      try {

        cropState.image = await loadImageElement(cropSource);

        resetCrop();

        drawCropCanvas();

        cropOverlay.classList.add("open");

      } catch (error) {

        showGeneralToast(error.message || T.dynamic.errors.imageFailed);

      }

    }

    function onCropCanvasPointerDown(event) {

      if (!cropState.image) return;

      isDraggingCrop = true;

      event.currentTarget.classList.add("dragging");

      event.currentTarget.setPointerCapture(event.pointerId);

      dragStart = { x: event.clientX, y: event.clientY, offsetX: cropState.offsetX, offsetY: cropState.offsetY };

    }

    function onCropCanvasPointerMove(event) {

      if (!isDraggingCrop || !cropState.image) return;

      const rect = event.currentTarget.getBoundingClientRect();

      const deltaX = ((event.clientX - dragStart.x) / rect.width) * 256;

      const deltaY = ((event.clientY - dragStart.y) / rect.height) * 256;

      cropState.offsetX = dragStart.offsetX + deltaX;

      cropState.offsetY = dragStart.offsetY + deltaY;

      drawCropCanvas();

    }

    function onCropCanvasPointerUp(event) {

      if (!isDraggingCrop) return;

      isDraggingCrop = false;

      event.currentTarget.classList.remove("dragging");

    }

    function onCropCanvasWheel(event) {

      if (!cropState.image) return;

      event.preventDefault();

      const next = Number(cropScale.value) + (event.deltaY < 0 ? 10 : -10);

      setCropScale(next);

    }

    async function resizeImageToSquare(file, size) {

      const image = await loadImageElement(file);

      const width = image.naturalWidth || image.width;

      const height = image.naturalHeight || image.height;

      const canvas = document.createElement("canvas");

      canvas.width = size;

      canvas.height = size;

      const context = canvas.getContext("2d");

      const scale = Math.min(size / width, size / height);

      const drawWidth = Math.max(1, Math.round(width * scale));

      const drawHeight = Math.max(1, Math.round(height * scale));

      context.clearRect(0, 0, size, size);

      context.drawImage(image, Math.round((size - drawWidth) / 2), Math.round((size - drawHeight) / 2), drawWidth, drawHeight);

      return canvas.toDataURL("image/png");

    }

    async function resizeImageToSquareFromDataUrl(dataUrl, size) {

      const response = await fetch(dataUrl);

      const blob = await response.blob();

      return resizeImageToSquare(blob, size);

    }

    function setAppnameLocked(locked) {

      formAppnameInput.readOnly = locked;

    }

    function markInvalid(input) {

      input.classList.add("invalid");

    }

    function clearInvalidForm() {

      form.querySelectorAll(".invalid").forEach((input) => input.classList.remove("invalid"));

    }

    function toggleTypeFields() {

      const type = formType || 'internal_port';

      const isExternal = type === 'external_url';

      document.querySelectorAll('.type-internal').forEach(el => el.classList.toggle('hidden', isExternal));

      document.querySelectorAll('.type-external').forEach(el => el.classList.toggle('hidden', !isExternal));

      urlInput.required = isExternal;

      portInput.required = !isExternal;

    }

    function clearForm() {      clearFileInput();

      revokeIconPreview();

      resetCropState();

      form.reset();

      currentEditAppname = null;

      formAppnameInput.value = "";

      setAppnameLocked(false);

      displayNameInput.value = "";

      urlInput.value = "";


      portInput.value = "";

      protocolInput.value = "http";

      pathInput.value = "/";

      openModeInput.value = "url";

      setDescriptionValue("");

      renderIconPreview(null);

    }

    function openFormModal(entry, type, opener) {

      clearForm();

      modalOpener = opener || (document.activeElement && document.activeElement !== document.body ? document.activeElement : addEntryButton);

      if (entry) {

        currentEditAppname = entry.appname || "";

        formAppnameInput.value = entry.appname || "";

        setAppnameLocked(true);

        displayNameInput.value = entry.displayName || "";

        portInput.value = entry.port || "";

        protocolInput.value = entry.protocol || "http";

        pathInput.value = entry.path || "/";

        openModeInput.value = entry.openMode || (entry.config && entry.config.openMode) || "iframe";

        urlInput.value = entry.url || (entry.config && entry.config.url) || "";

        setDescriptionValue(entry.description || "");

      }

      formType = type || (entry && entry.type) || (entry && entry.config && entry.config.type) || 'internal_port';

      toggleTypeFields();

      renderIconPreview(entry);

      entryFormTitle.textContent = FORM_TYPE_LABELS[formType] || formType;

      overlay.scrollTop = 0;

      overlay.classList.add("open");

      // 不自动聚焦输入框，避免移动端弹窗时弹出键盘

    }

    function closeModal() {

      overlay.classList.remove("open");

      cropOverlay.classList.remove("open");

      revokeIconPreview();

      clearFileInput();

      const focusTarget = modalOpener && modalOpener.isConnected ? modalOpener : addEntryButton;

      modalOpener = null;

      if (focusTarget && typeof focusTarget.focus === "function") focusTarget.focus();

    }

    // 新建流程第一步：选择快捷方式类型。选完后进入锁定该类型的表单弹窗。
    function openTypePicker() {

      typePickerOverlay.classList.add("open");

    }

    function closeTypePicker() {

      typePickerOverlay.classList.remove("open");

      if (addEntryButton && typeof addEntryButton.focus === "function") addEntryButton.focus();

    }

    function getFocusableModalElements() {

      const selector = 'button:not([disabled]), a[href], input:not([disabled]), select:not([disabled]), textarea:not([disabled]), [contenteditable="true"], [tabindex]:not([tabindex="-1"])';

      return Array.from(entryModal.querySelectorAll(selector)).filter((element) => element.getClientRects().length > 0);

    }

    function onModalFocusTrapKeydown(event) {

      if (event.key !== "Tab") return;

      const focusables = getFocusableModalElements();

      if (!focusables.length) return;

      const first = focusables[0];

      const last = focusables[focusables.length - 1];

      const active = document.activeElement;

      if (!entryModal.contains(active)) {

        event.preventDefault();

        first.focus();

        return;

      }

      if (event.shiftKey && active === first) {

        event.preventDefault();

        last.focus();

        return;

      }

      if (!event.shiftKey && active === last) {

        event.preventDefault();

        first.focus();

      }

    }

    function normalizeEntry(entry) {

      const config = entry.config && typeof entry.config === "object" ? entry.config : {};

      return {

        ...entry,

        protocol: entry.protocol || config.protocol || "http",

        port: entry.port || config.port || "",

        path: entry.path || config.path || "/",

        openMode: entry.openMode || config.openMode || (entry.type === "external_url" || (entry.config && entry.config.type) === "external_url" ? "url" : "iframe"),

        url: entry.url || config.url || "",

        type: entry.type || config.type || "internal_port",

        url: entry.url || config.url || "",

      };

    }



    // 六点手柄：单纯的点阵比「≡」更像可拖拽的抓手，也不会被误认成菜单按钮。
    const DRAG_HANDLE_ICON = '<svg viewBox="0 0 12 18" aria-hidden="true" draggable="false"><circle cx="4" cy="4" r="1.4"></circle><circle cx="8" cy="4" r="1.4"></circle><circle cx="4" cy="9" r="1.4"></circle><circle cx="8" cy="9" r="1.4"></circle><circle cx="4" cy="14" r="1.4"></circle><circle cx="8" cy="14" r="1.4"></circle></svg>';

    // 类型图标：与「新增配置」类型选择弹窗里的一致，列表里放在目标值前面，
    // 让不同类型的入口一眼能分清，不必回去看名字猜。
    const TYPE_ICONS = {

      internal_port: '<rect x="2" y="3" width="20" height="7" rx="2"></rect><rect x="2" y="14" width="20" height="7" rx="2"></rect><path d="M6 6.5h.01M6 17.5h.01"></path>',

      external_url: '<path d="M10 13a5 5 0 0 0 7.54.54l3-3a5 5 0 0 0-7.07-7.07l-1.72 1.71"></path><path d="M14 11a5 5 0 0 0-7.54-.54l-3 3a5 5 0 0 0 7.07 7.07l1.71-1.71"></path>',


    };

    function renderTargetCell(entry) {

      const target = entry.type === "external_url"
        ? (entry.url || "")
          : (entry.port || "");

      const icon = TYPE_ICONS[entry.type] || TYPE_ICONS.internal_port;

      return `<td class="entry-cell entry-target" title="${escapeHtml(target)}"><span class="target-value"><svg class="target-icon" viewBox="0 0 24 24" aria-hidden="true">${icon}</svg><span class="target-text">${escapeHtml(target)}</span></span></td>`;

    }

    function renderActions(entry) {

      const id = escapeHtml(entry.appname);

      const deployment = entry.deployment || entry.status || "undeployed";

      const parts = [];

      if (deployment === "undeployed") {

        parts.push(`<button type="button" class="action-button primary" data-deploy="${id}">${T.dynamic.action.deploy}</button>`);

      } else if (deployment === "outdated") {

        parts.push(`<button type="button" class="action-button primary" data-deploy="${id}">${T.dynamic.action.update}</button>`);

        parts.push(`<button type="button" class="action-button ghost-danger" data-uninstall="${id}">${T.dynamic.action.uninstall}</button>`);

      } else if (entry.runtime === "running") {

        parts.push(`<button type="button" class="action-button" data-disable="${id}">${T.dynamic.action.disable}</button>`);

        parts.push(`<button type="button" class="action-button ghost-danger" data-uninstall="${id}">${T.dynamic.action.uninstall}</button>`);

      } else {

        parts.push(`<button type="button" class="action-button primary" data-enable="${id}">${T.dynamic.action.enable}</button>`);

        parts.push(`<button type="button" class="action-button ghost-danger" data-uninstall="${id}">${T.dynamic.action.uninstall}</button>`);

      }

      parts.push(`<button type="button" class="action-button" data-edit="${id}">${T.dynamic.action.edit}</button>`);

      parts.push(`<button type="button" class="action-button ghost-danger" data-delete="${id}">${T.dynamic.action.delete}</button>`);

      return `<div class="entry-actions">${parts.join("")}</div>`;

    }

    // 状态圆点：把原来的文字标签压缩成左侧一列颜色点，颜色即状态，
    // 完整文案放 title/aria-label 里，悬停或读屏仍能拿到。
    // 灰=未部署/未创建，绿=已部署，黄=待更新/桌面项丢失，红=未知状态兜底。
    const STATUS_DOT_TONES = {

      undeployed: "idle",

      deployed: "ok",

      outdated: "warn",


    };

    function renderStatusDot(entry) {

      const deployment = entry.deployment || entry.status || "undeployed";

      const label = DEPLOY_LABELS[deployment] || deployment;

      const tone = STATUS_DOT_TONES[deployment] || "danger";

      const runtime = entry.runtime || "";

      const runtimeLabel = runtime && deployment !== "undeployed" ? RUNTIME_LABELS[runtime] : "";

      const text = runtimeLabel ? `${label} · ${runtimeLabel}` : label;

      // 已停止用空心圆点表示：和"未部署"的灰色实心点区分开，又不用多一种颜色。
      const stopped = runtime === "stopped" ? " is-stopped" : "";

      return `<span class="status-dot status-dot-${tone}${stopped}" role="img" aria-label="${escapeHtml(text)}" title="${escapeHtml(text)}"></span>`;

    }

    function renderEntries(entries) {

      entryCount.textContent = formatText(T.dynamic.items, { count: entries.length });

      listNotice.classList.toggle("hidden", entries.length > 0);

      listWrap.classList.toggle("hidden", entries.length === 0);

      listLoading.classList.add("hidden");

      entryRows.innerHTML = entries

        .map((entry) => {

          return `<tr data-appname="${escapeHtml(entry.appname)}">

            <td class="drag-handle-cell"><span class="drag-handle" role="button" tabindex="0" draggable="false" aria-label="${escapeHtml(T.dynamic.reorderHandle)}" title="${escapeHtml(T.dynamic.reorderHandle)}">${DRAG_HANDLE_ICON}</span></td>

            <td class="status-dot-cell">${renderStatusDot(entry)}</td>

            <td class="entry-cell entry-name-cell" title="${escapeHtml(entry.displayName)}"><span class="entry-title">${escapeHtml(entry.displayName)}</span></td>

            ${renderTargetCell(entry)}

            <td class="entry-cell col-center">${renderActions(entry)}</td>

          </tr>`;

        })

        .join("");

    }

    async function fetchEntries(options = {}) {

      // 拖拽或保存顺序期间不能重绘：整表 innerHTML 重绘会把正在拖的行
      // 从 DOM 里摘掉，也会让用户刚排好的顺序跳回服务端旧值。
      // 在途请求返回后要再判断一次：请求可能早于拖拽开始。
      if (!options.force && (isReordering || isSavingOrder)) return currentEntries;

      if (currentEntries.length > 0) {
        // 已有数据时不显示"加载中..."，避免撑开空间导致表格抖动
        listNotice.classList.add("hidden");
        listLoading.classList.add("hidden");
        listWrap.classList.remove("hidden");
      } else {
        listNotice.classList.add("hidden");
        listLoading.classList.remove("hidden");
        listWrap.classList.remove("hidden");
      }

      const response = await fetchJsonWithTimeout(`${API_BASE}/api/entries`, { timeoutMs: 30000 });

      const data = await response.json();

      if (!response.ok) throw new Error(data.error || T.dynamic.errors.readFallback);

      if (!options.force && (isReordering || isSavingOrder)) return currentEntries;

      currentEntries = (data.entries || []).map((entry) => normalizeEntry(entry));

      renderEntries(currentEntries);


      return currentEntries;

    }



    // 配置列表排序。拖动过程中 DOM 顺序就是唯一事实来源，松手或按键结束
    // 后才把顺序同步到 currentEntries 并发给服务端。

    function currentOrderFromDom() {
      return Array.from(entryRows.children).map((row) => row.getAttribute("data-appname") || "");
    }

    function rowFromEvent(event) {
      const target = event.target instanceof Element ? event.target.closest("tr") : null;
      return target && target.parentElement === entryRows ? target : null;
    }

    function hasOrderChanged(order) {
      if (order.length !== currentEntries.length) return true;
      return order.some((appname, index) => currentEntries[index].appname !== appname);
    }

    function applyOrderToCache(order) {
      const byName = new Map(currentEntries.map((entry) => [entry.appname, entry]));
      const next = order.map((appname) => byName.get(appname));
      if (next.every(Boolean)) currentEntries = next;
    }

    function announceOrderPosition(row) {
      if (!orderAnnounce) return;
      const position = Array.prototype.indexOf.call(entryRows.children, row) + 1;
      orderAnnounce.textContent = formatText(T.dynamic.reorderAnnounce, { position });
    }

    function resetRowDraggable() {
      if (rowDragState && rowDragState.row) rowDragState.row.draggable = false;
      rowDragState = null;
    }

    function finishRowDrag(event) {
      const state = rowDragState;
      if (!state) {
        isReordering = false;
        return;
      }
      const order = currentOrderFromDom();
      const changed = Array.isArray(state.originalOrder) && order.join("\n") !== state.originalOrder.join("\n");
      // 只有真正落在行上（drop 被拦截）才算提交；Esc、拖出表格松手都不会有 drop。
      const cancelled = !state.dropped;
      if (state.row) {
        state.row.classList.remove("is-dragging");
        state.row.draggable = false;
      }
      rowDragState = null;
      isReordering = false;
      if (!changed) return;
      if (cancelled) {
        // Esc 或拖到表格外：放回拖拽前的顺序，不写盘。
        renderEntries(currentEntries);
        return;
      }
      requestOrderSave();
    }

    function persistOrder() {
      const order = currentOrderFromDom();
      if (order.length !== currentEntries.length || !hasOrderChanged(order)) return Promise.resolve();
      // 先乐观更新缓存；保存失败时下一次静默刷新会以服务端为准纠正。
      applyOrderToCache(order);
      return fetchJsonWithTimeout(`${API_BASE}/api/entries`, {
        method: "PUT",
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify({ order }),
        timeoutMs: 15000,
      }).then(async (response) => {
        const data = await response.json().catch(() => ({}));
        if (!response.ok) throw new Error(data.detail || data.error || T.dynamic.messages.reorderFailed);
        if (!Array.isArray(data.entries) || isReordering) return;
        // 保存期间顺序又变过：这次响应已经过期，别按旧顺序覆盖 DOM 和缓存，
        // 交给排队中的下一次保存。
        const domOrder = currentOrderFromDom();
        if (domOrder.join("\n") !== order.join("\n")) return;
        const nextEntries = data.entries.map((entry) => normalizeEntry(entry));
        currentEntries = nextEntries;
        // 顺序与 DOM 一致时不重绘：键盘排序后重绘会把焦点从手柄上抢走。
        if (domOrder.join("\n") !== nextEntries.map((entry) => entry.appname).join("\n")) {
          renderEntries(currentEntries);
        }
      }).catch((error) => {
        showGeneralToast(userMessage(ERROR_CODES.SAVE, T.dynamic.messages.reorderFailed, errorDetail(error)));
        // 服务端可能已经有别的改动，强制拉一次真实顺序；拖拽中先跳过，
        // 松手后的静默刷新会纠正。
        if (!isReordering) fetchEntries({ force: true }).catch(() => {});
      });
    }

    function requestOrderSave() {
      orderSavePending += 1;
      isSavingOrder = true;
      orderSaveChain = orderSaveChain.then(() => persistOrder()).catch(() => {}).finally(() => {
        orderSavePending -= 1;
        if (orderSavePending === 0) isSavingOrder = false;
      });
      return orderSaveChain;
    }

    function scheduleKeyboardOrderSave() {
      isReordering = true;
      if (orderSaveTimer) clearTimeout(orderSaveTimer);
      orderSaveTimer = setTimeout(() => {
        orderSaveTimer = null;
        if (rowDragState && rowDragState.originalOrder) {
          // 真正拖拽中：交给 dragend 保存，别把中途顺序写进去。
          return;
        }
        // 只剩「按下手柄但没开始拖」的残留状态，顺手清掉。
        resetRowDraggable();
        isReordering = false;
        requestOrderSave();
      }, 250);
    }

    // 列表刷新入口。分两层：
    //   refreshList      — 去重 + 仅在标签页可见且没有操作弹窗时刷新。
    //                      大多数触发场景（编辑点击、定时器、visibilitychange）用这个。
    //   forceRefreshList — 跳过可见性检查。在 CRUD 操作之后使用：
    //                      响应已经知道是新状态，即使弹窗短暂可见也必须重新渲染。
    // showOnError 布尔值控制刷新失败时是否向用户显示提示。
    // 大多数调用点保持静默；只有初始加载和操作后的刷新传 true，
    // 因为只有这两个时刻弹 toast 才真正有用。
    function shouldRefreshList() {
      if (isReordering || isSavingOrder) return false;
      return document.visibilityState === "visible" && !operationOverlay.classList.contains("open");
    }

    async function forceRefreshList(showOnError = false) {
      if (refreshEntriesPromise) return refreshEntriesPromise;
      refreshEntriesPromise = fetchEntries().catch((error) => {
        if (showOnError) {
          showNotice(userMessage(ERROR_CODES.READ, T.dynamic.errors.refreshFallback, errorDetail(error)), "warn");
        }
        throw error;
      }).finally(() => {
        refreshEntriesPromise = null;
      });
      return refreshEntriesPromise;
    }

    async function refreshList(showOnError = false) {
      if (!shouldRefreshList()) return currentEntries;
      if (refreshEntriesPromise) return refreshEntriesPromise;
      if (showOnError) {
        // 初始加载：列表读取失败是唯一一个用户看到空页面、
        // 我们必须解释原因的时刻。用通用 toast（顶部居中）
        // 而不是窄条 notice。
        return fetchEntries().catch((error) => {
          showGeneralToast(userMessage(ERROR_CODES.READ, T.dynamic.errors.readListFallback, errorDetail(error)));
          throw error;
        });
      }
      return forceRefreshList();
    }

    // 便利封装：约 11 个调用点在用户可见操作（保存、删除、恢复、裁剪等）
    // 之后只需要静默刷新。没有这个封装，每个调用点都要重复写
    // ".catch(() => {})" 来吞掉重新抛出的错误。
    function refreshListSilently() {
      forceRefreshList().catch(() => {});
    }



    function startRefreshTimer() {

      if (refreshTimer) clearInterval(refreshTimer);

      refreshTimer = setInterval(() => {

        refreshList().catch(() => {});

      }, 10000);

    }



    function onVisibilityRefresh() {

      if (document.visibilityState === "visible") {

        refreshList().catch(() => {});

      }

    }



    document.addEventListener("visibilitychange", onVisibilityRefresh);

    startRefreshTimer();

    // 启动时恢复明暗主题。之前 initTheme 定义了却从未调用，所以重开总是浅色。
    initTheme();

    refreshList(true).catch(() => {});

    function loadImageElement(file) {

      return new Promise((resolve, reject) => {

        const url = URL.createObjectURL(file);

        const image = new Image();

        image.onload = () => {

          URL.revokeObjectURL(url);

          resolve(image);

        };

        image.onerror = () => {

          URL.revokeObjectURL(url);

          reject(new Error(T.dynamic.errors.imageFailed));

        };

        image.src = url;

      });

    }

    async function saveEntry(event) {

      event.preventDefault();

      clearInvalidForm();

      const payload = {

        appname: formAppnameInput.value.trim(),

        displayName: displayNameInput.value.trim(),

        description: getDescriptionValue(),

      };

      if (!payload.appname) {

        markInvalid(formAppnameInput);

        showGeneralToast(T.dynamic.errors.requiredAppname);

        return;

      }

      if (!APPNAME_PATTERN.test(payload.appname)) {

        markInvalid(formAppnameInput);

        showGeneralToast(T.dynamic.errors.appnameInvalid);

        return;

      }

      if (!payload.displayName) {

        markInvalid(displayNameInput);

        showGeneralToast(T.dynamic.errors.requiredDisplayName);

        return;

      }

      if (payload.displayName.length > MAX_DISPLAY_NAME) {

        markInvalid(displayNameInput);

        showGeneralToast(formatText(T.dynamic.errors.displayNameMax, { max: MAX_DISPLAY_NAME }));

        return;

      }

      payload.type = formType || 'internal_port';

      if (payload.type === 'external_url') {

        const rawUrl = urlInput.value.trim();

        if (!rawUrl) {

          markInvalid(urlInput);

          showGeneralToast(T.dynamic.errors.requiredUrl);

          return;

        }

        payload.url = rawUrl;

        payload.config = {

          url: rawUrl,

          openMode: openModeInput.value,

        };

      } else {

        const port = portInput.value.trim();

        if (!port) {

          markInvalid(portInput);

          showGeneralToast(T.dynamic.errors.requiredPort);

          return;

        }

        payload.config = {

          port: port,

          protocol: protocolInput.value,

          path: pathInput.value.trim() || "/",

          openMode: openModeInput.value,

        };

      }

      if (pendingIcon === "default") {

        payload.removeIcon = true;

      } else if (pendingIcon && typeof pendingIcon === "object") {

        try {

          // 64 与 256 两个文件都存 256px 内容。图标只按 256 展示，缩到 64 只会让
          // 子应用里的小图标变糊；64 那份只是文件名不同，内容与 256 完全一致。

          const iconData = pendingIcon.dataUrl
            ? await resizeImageToSquareFromDataUrl(pendingIcon.dataUrl, 256)
            : await resizeImageToSquare(pendingIcon.file, 256);

          payload.icon64Data = iconData;

          payload.icon256Data = iconData;

        } catch (error) {

          showGeneralToast(userMessage(ERROR_CODES.ICON, T.dynamic.messages.iconFailed, errorDetail(error)));

          return;

        }

      }

      if (currentEditAppname) {

        payload.appname = currentEditAppname;

      }


      const hasIcon = !!(payload.icon64Data || payload.icon256Data);

      const requestUrl = buildSaveRequestUrl(payload);

      const requestBody = JSON.stringify(payload);

      console.info("shortcuts save request", "POST", requestUrl, requestBody.length, { hasIcon, removeIcon: !!payload.removeIcon });

      const saveButton = document.querySelector("#saveButton");

      const previousSaveLabel = saveButton.textContent;

      saveButton.disabled = true;

      saveButton.textContent = T.dynamic.action.saving;

      try {

        const response = await fetchJsonWithTimeout(requestUrl, {

          method: "POST",

          headers: { "Content-Type": "application/json" },

          body: requestBody,

          timeoutMs: 30000,

        });

        if (!response.ok) {

          let errorData = {};

          let rawErrorText = "";

          try {

            rawErrorText = await response.text();

            try {

              errorData = JSON.parse(rawErrorText);

            } catch (parseError) {

              errorData = { error: ERROR_CODES.SAVE, detail: `Response is not JSON: ${rawErrorText.slice(0, 500)}` };

            }

          } catch (readError) {

            errorData = { error: ERROR_CODES.SAVE, detail: `Failed to read response: ${readError.message}` };

          }

          console.warn(

            "shortcuts save response",

            response.status,

            response.statusText,

            {

              contentType: response.headers && response.headers.get ? response.headers.get("content-type") : "",

              contentLength: response.headers && response.headers.get ? response.headers.get("content-length") : "",

              bodyBytes: rawErrorText.length,

              body: rawErrorText.slice(0, 1000),

            },

          );

          const failureDetail = saveFailureDetail(response, errorData);

          showGeneralToast(userMessage(errorData.error || ERROR_CODES.SAVE, T.dynamic.messages.saveFailed, failureDetail, failureDetail));

          return;

        }

        const data = await response.json();

        const changed = data.changed === undefined ? Boolean(currentEditAppname) : Boolean(data.changed);

        // 名字带上，与「已部署 xx」「已删除 xx」一致：
        // 更新从本地列表取（响应前已有条目），新建从响应 entry 或表单输入取。
        const saveName = (currentEditAppname ? entryDisplayName(currentEditAppname, currentEditAppname) : "")
          || (data.entry && data.entry.displayName) || payload.displayName || "";

        const saveMessage = currentEditAppname
          ? (changed ? formatText(T.dynamic.messages.saveUpdated, { target: saveName }) : T.dynamic.messages.saveUnchanged)
          : formatText(T.dynamic.messages.saveCreated, { target: saveName });

        showGeneralToast(saveMessage);

        closeModal();

        refreshListSilently();

      } catch (error) {

        showGeneralToast(userMessage(ERROR_CODES.SAVE, T.dynamic.messages.saveFailed, errorDetail(error)));

      } finally {

        saveButton.disabled = false;

        saveButton.textContent = previousSaveLabel;

      }

    }

    async function deployEntry(id, button) {

      const noun = button.textContent.includes(T.dynamic.action.update) ? T.dynamic.action.update : T.dynamic.action.deploy;

      button.disabled = true;

      showOperationOverlay(formatText(T.dynamic.operation.running, { name: noun }));

      try {

        const response = await fetchJsonWithTimeout(`${API_BASE}/api/entries/${encodeURIComponent(id)}/deploy`, { method: "POST" });

        const responseText = await response.text();

        console.info("shortcuts deploy response", response.status, response.statusText, responseText.slice(0, 1000));

        let data;

        try { data = JSON.parse(responseText); } catch (_) { data = { error: ERROR_CODES.DEPLOY, detail: `Response is not JSON: ${responseText.slice(0, 200)}` }; }

        if (!response.ok) {

          console.warn("shortcuts deploy response", response.status, response.statusText, data);

          showNotice(userMessage(ERROR_CODES.DEPLOY, formatText(T.dynamic.messages.deployFailed, { action: noun }), data.detail), "warn");

          return;

        }

        if (data.disabled) {

          showNotice(userMessage(ERROR_CODES.DISABLED, T.dynamic.messages.deployDisabled), "warn");

          return;

        }

        const name = entryDisplayName(id, id);

        if (data.deployed) {

          // 已是最新版本时后端会带 DEPLOY_SKIPPED：这次点击其实没有执行部署，
          // 再弹「已部署 MoviePilot」会让人以为刚刚又装了一遍。
          if (data.code === "DEPLOY_SKIPPED") {

            showNotice(formatText(T.dynamic.messages.deploySkipped, { target: name }), "ok");

            refreshListSilently();

            return;

          }

          showNotice(data.restoredStopped

            ? formatText(T.dynamic.messages.deploySuccessStopped, { name: noun, target: name })

            : formatText(T.dynamic.messages.deploySuccess, { name: noun, target: name }), "ok");

          refreshListSilently();

          return;

        }

        if (data.code === ERROR_CODES.TIMEOUT) {

          showNotice(userMessage(ERROR_CODES.TIMEOUT, T.dynamic.messages.deployTimeout, data.output), "warn");

          refreshListSilently();

          return;

        }

        if (data.verificationWarning) {

          showNotice(userMessage(ERROR_CODES.DEPLOY, formatText(T.dynamic.messages.deployNotEffective, { action: noun }), data.output), "warn");

          refreshListSilently();

          return;

        }

        showNotice(data.restoredStopped

          ? formatText(T.dynamic.messages.deploySuccessStopped, { name: noun, target: name })

          : formatText(T.dynamic.messages.deploySuccess, { name: noun, target: name }), "ok");

        refreshListSilently();

      } catch (error) {

        console.warn("shortcuts deploy error", error.message);

        showNotice(userMessage(ERROR_CODES.DEPLOY, formatText(T.dynamic.messages.deployFailed, { action: noun }), errorDetail(error)), "warn");

      } finally {

        hideOperationOverlay();

        button.disabled = false;

      }

    }


    async function uninstallEntry(id, button) {

      button.disabled = true;

      showOperationOverlay(formatText(T.dynamic.operation.running, { name: T.dynamic.action.uninstall }));

      try {

        const response = await fetchJsonWithTimeout(`${API_BASE}/api/entries/${encodeURIComponent(id)}/uninstall`, { method: "POST" });

        const data = await response.json();

        if (!response.ok) {

          showNotice(userMessage(ERROR_CODES.UNINSTALL, T.dynamic.messages.uninstallFailed, data.detail), "warn");

          return;

        }

        const uninstallCode = data.code || "UNINSTALL_DONE";

        // 后端从不返回 action: "failed"（只会是 "uninstalled" /
        // "uninstall-unverified"，或者直接就是 code 本身），原先按 action 判断
        // 使「卸载与停用都未成功」这类结果也套上绿色成功样式。严重程度改由
        // code 决定：只有 UNINSTALL_DONE 算成功，其余都是需要留意的结果。
        // 名字从本地列表取（entryDisplayName），提示里带上它才知道卸的是哪条。
        const uninstallMessage = formatText(
          localMessage(uninstallCode, T.dynamic.messages.uninstallFailed),
          { target: entryDisplayName(id, id) },
        );

        if (uninstallCode === "UNINSTALL_DONE") {

          showNotice(uninstallMessage, "ok");

        } else {

          showNotice(userMessage(ERROR_CODES.UNINSTALL, uninstallMessage, data.output), "warn");

        }

        refreshListSilently();

      } catch (error) {

        showNotice(userMessage(ERROR_CODES.UNINSTALL, T.dynamic.messages.uninstallFailed, errorDetail(error)), "warn");

      } finally {

        hideOperationOverlay();

        button.disabled = false;

      }

    }

    async function setRuntime(id, action, button) {

      const label = action === "start" ? T.dynamic.action.enable : T.dynamic.action.disable;

      button.disabled = true;

      showOperationOverlay(formatText(T.dynamic.operation.running, { name: label }));

      try {

        const response = await fetchJsonWithTimeout(`${API_BASE}/api/entries/${encodeURIComponent(id)}/${action}`, { method: "POST" });

        const data = await response.json();

        if (!response.ok) {

          showNotice(userMessage(ERROR_CODES.RUNTIME, formatText(T.dynamic.messages.runtimeFailed, { action: label }), data.detail), "warn");

          return;

        }

        // 名字从本地列表取（entryDisplayName）：响应里没有顶层的
        // displayName/appname，捷径名字在 entry.displayName。早先取不到就退回
        // 动作标签，提示才会变成「已停用：启用」这种读不通的句子。
        showRuntimeSuccess(action, entryDisplayName(id, id));

        refreshListSilently();

      } catch (error) {

        showNotice(userMessage(ERROR_CODES.RUNTIME, formatText(T.dynamic.messages.runtimeFailed, { action: label }), errorDetail(error)), "warn");

      } finally {

        hideOperationOverlay();

        button.disabled = false;

      }

    }

    async function deleteEntry(id, button) {

      if (!window.confirm(T.dynamic.errors.deleteConfirm)) return;

      if (button) button.disabled = true;

      // 名字必须在删除前取：删除后列表一刷新，条目就不在 currentEntries 里了，
      // 提示里就带不上「删的是哪条」。
      const name = entryDisplayName(id, id);

      // 删除前可能有一个还在途的列表刷新；等它结束再刷，
      // 否则它拿到的是删除前的旧数据，会把刚删掉的条目重新渲染出来。

      const refreshAfterDelete = async () => {

        if (refreshEntriesPromise) {

          try {

            await refreshEntriesPromise;

          } catch (_) {}

        }

        await refreshListSilently();

      };

      showOperationOverlay(formatText(T.dynamic.operation.running, { name: T.dynamic.action.delete }));

      try {

        const response = await fetchJsonWithTimeout(`${API_BASE}/api/entries/${encodeURIComponent(id)}`, { method: "DELETE" });

        // 网关可能返回非 JSON（登录页、错误页），解析失败也要给出可诊断的错误。

        const text = await response.text();

        let data = {};

        try {

          data = text ? JSON.parse(text) : {};

        } catch (parseError) {

          data = { error: ERROR_CODES.DELETE, detail: `Response is not JSON: ${text.slice(0, 200)}` };

        }

        if (!response.ok) {

          hideOperationOverlay();

          showGeneralToast(userMessage(ERROR_CODES.DELETE, T.dynamic.messages.deleteFailed, data.detail));

          return;

        }

        // 先关掉 loading 再提示：spinner 还在转时弹出「已删除」会让人以为
        // 操作还没结束。finally 里保留一次调用兜底，remove class 与恢复
        // 按钮都是幂等的。

        hideOperationOverlay();

        showGeneralToast(formatText(T.dynamic.messages.deleted, { target: name }));

        await refreshAfterDelete();

        if (data.stillInstalled) {

          // 卸载命令的原始输出按行展示，取前 3 行非空行。
          // 压成一行会把换行变成空格，多行输出读起来像一句被空格糊住的话。

          const uninstallDetail = String(data.uninstallDetail || "")
            .split(/\r?\n/)
            .map((line) => line.trim())
            .filter(Boolean)
            .slice(0, 3)
            .join("\n");

          showNotice(userMessage(ERROR_CODES.DELETE_CLEANUP, formatText(T.dynamic.messages.deleteCleanup, { target: name }), uninstallDetail), "warn");

        }

      } catch (error) {

        if (error && error.message === ERROR_MESSAGES[ERROR_CODES.TIMEOUT]) {

          // 捷径已经落盘；超时只说明子应用卸载还没跑完，重试删除反而报"不存在"。

          hideOperationOverlay();

          showGeneralToast(formatText(T.dynamic.messages.deleted, { target: name }));

          await refreshAfterDelete();

          showNotice(formatText(T.dynamic.messages.deleteUninstallPending, { target: name }), "warn");

        } else {

          hideOperationOverlay();

          showGeneralToast(userMessage(ERROR_CODES.DELETE, T.dynamic.messages.deleteFailed, errorDetail(error)));

        }

      } finally {

        hideOperationOverlay();

        if (button) button.disabled = false;

      }

    }

    form.addEventListener("submit", saveEntry);

    form.addEventListener("input", (event) => {

      if (event.target && event.target.classList) event.target.classList.remove("invalid");

    });

    cancelButton.addEventListener("click", closeModal);

    entryFormCloseButton.addEventListener("click", closeModal);

    overlay.addEventListener("keydown", onModalFocusTrapKeydown);

    addEntryButton.addEventListener("click", openTypePicker);

    typePickerGrid.addEventListener("click", (event) => {

      const card = event.target.closest("[data-type]");

      if (!card) return;

      const type = card.getAttribute("data-type");

      closeTypePicker();

      openFormModal(null, type, addEntryButton);

    });

    typePickerCloseButton.addEventListener("click", closeTypePicker);

    themeButton.addEventListener("click", toggleTheme);

    displayIconInput.addEventListener("change", async () => {

      const file = displayIconInput.files && displayIconInput.files[0];

      if (!file) {

        // 取消选择：放弃本次未保存的新文件，回到已保存状态

        pendingIcon = "keep";

        renderPersistedIcon(currentEditAppname);

        return;

      }

      pendingIcon = { file: file, dataUrl: "" };

      renderPendingIconPreview();

      await openCropModal();

    });

    iconCropButton.addEventListener("click", (event) => { event.preventDefault(); event.stopPropagation(); openCropModal(); });

    iconRestoreButton.addEventListener("click", (event) => { event.preventDefault(); event.stopPropagation(); restoreSavedIcon(); });

    iconClearButton.addEventListener("click", (event) => { event.preventDefault(); event.stopPropagation(); resetIconToDefault(); });

    cropCloseButton.addEventListener("click", () => cropOverlay.classList.remove("open"));

    cropApplyButton.addEventListener("click", applyCrop);

    cropResetButton.addEventListener("click", resetCrop);

    cropScale.addEventListener("input", () => setCropScale(Number(cropScale.value)));

    cropCanvas.addEventListener("pointerdown", onCropCanvasPointerDown);

    cropCanvas.addEventListener("pointermove", onCropCanvasPointerMove);

    cropCanvas.addEventListener("pointerup", onCropCanvasPointerUp);

    cropCanvas.addEventListener("pointercancel", onCropCanvasPointerUp);

    cropCanvas.addEventListener("wheel", onCropCanvasWheel, { passive: false });



    formAppnameInput.addEventListener("input", () => {

      if (formAppnameInput.readOnly) return;

      formAppnameInput.value = formAppnameInput.value.toLowerCase().replace(/[^a-z0-9-]/g, "");

    });

    entryRows.addEventListener("mousedown", (event) => {
      if (event.button !== 0) return;
      const handle = event.target instanceof Element ? event.target.closest(".drag-handle") : null;
      if (!handle) return;
      const row = handle.closest("tr");
      if (!row || row.parentElement !== entryRows) return;
      // HTML5 拖拽要求 dragstart 前元素已经是 draggable。只在手柄上按下时
      // 临时打开，避免整行文字和操作按钮也能被拖走。
      row.draggable = true;
      rowDragState = { row, originalOrder: null };
    });

    document.addEventListener("mouseup", () => {
      // 只按下没拖动时不会有 dragend，这里复位，别让整行保持可拖状态。
      if (!rowDragState || isReordering) return;
      resetRowDraggable();
    });

    entryRows.addEventListener("dragstart", (event) => {
      const row = event.target instanceof Element ? event.target.closest("tr") : null;
      if (!rowDragState || !row || rowDragState.row !== row) {
        event.preventDefault();
        return;
      }
      rowDragState.originalOrder = currentOrderFromDom();
      rowDragState.dropped = false;
      row.classList.add("is-dragging");
      isReordering = true;
      if (event.dataTransfer) {
        event.dataTransfer.effectAllowed = "move";
        // 部分 Chromium 内嵌宿主只有在 setData 后才会真正开始拖拽。
        event.dataTransfer.setData("text/plain", row.getAttribute("data-appname") || "");
      }
    });

    entryRows.addEventListener("dragover", (event) => {
      if (!rowDragState || !rowDragState.row) return;
      event.preventDefault();
      if (event.dataTransfer) event.dataTransfer.dropEffect = "move";
      const overRow = rowFromEvent(event);
      const dragged = rowDragState.row;
      if (!overRow || overRow === dragged) return;
      // 越过目标行中线就换位，拖动过程中即时反馈，松手不需要再算一次。
      const rect = overRow.getBoundingClientRect();
      const insertAfter = event.clientY > rect.top + rect.height / 2;
      entryRows.insertBefore(dragged, insertAfter ? overRow.nextSibling : overRow);
    });

    entryRows.addEventListener("drop", (event) => {
      if (!rowDragState) return;
      // 不拦截的话浏览器会按默认行为导航到拖拽数据。
      event.preventDefault();
      rowDragState.dropped = true;
    });

    document.addEventListener("dragend", finishRowDrag);

    entryRows.addEventListener("keydown", (event) => {
      if (event.key !== "ArrowUp" && event.key !== "ArrowDown") return;
      const handle = event.target instanceof Element ? event.target.closest(".drag-handle") : null;
      if (!handle) return;
      const row = handle.closest("tr");
      if (!row) return;
      const sibling = event.key === "ArrowUp" ? row.previousElementSibling : row.nextElementSibling;
      if (!sibling) return;
      event.preventDefault();
      if (event.key === "ArrowUp") entryRows.insertBefore(row, sibling);
      else entryRows.insertBefore(sibling, row);
      handle.focus();
      announceOrderPosition(row);
      scheduleKeyboardOrderSave();
    });

    entryRows.addEventListener("click", async (event) => {

      const target = event.target.closest("button");

      if (!target) return;

      const deployId = target.getAttribute("data-deploy");

      const uninstallId = target.getAttribute("data-uninstall");

      const enableId = target.getAttribute("data-enable");

      const disableId = target.getAttribute("data-disable");

      const editId = target.getAttribute("data-edit");

      const deleteId = target.getAttribute("data-delete");


      if (deployId) {

        await deployEntry(deployId, target);

        return;

      }

      if (uninstallId) {

        await uninstallEntry(uninstallId, target);

        return;

      }

      if (enableId) {

        await setRuntime(enableId, "start", target);

        return;

      }

      if (disableId) {

        await setRuntime(disableId, "stop", target);

        return;

      }

      if (deleteId) {

        await deleteEntry(deleteId, target);

        return;

      }

      if (editId) {

        const entries = await refreshList();

        const entry = entries.find((item) => item.appname === editId);

        if (entry) openFormModal(entry);

      }

    });

    cropOverlay.addEventListener("click", (event) => {

      if (event.target === cropOverlay) cropOverlay.classList.remove("open");

    });

    document.addEventListener("keydown", (event) => {

      if (event.key === "Escape" && cropOverlay.classList.contains("open")) cropOverlay.classList.remove("open");

      if (event.key === "Escape" && typePickerOverlay.classList.contains("open")) closeTypePicker();

    });

