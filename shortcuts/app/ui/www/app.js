    initLocalizedHtml();



    const DEPLOY_LABELS = T.dynamic.status;

    const FOLDER_STATUS_LABELS = {

      undeployed: T.dynamic.status.folderUndeployed,

      deployed: T.dynamic.status.folderDeployed,

      desktop_missing: T.dynamic.status.desktopMissing,

    };

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

    const folderPathInput = document.querySelector("#folderPath");

    const folderBrowseButton = document.getElementById("folderBrowseButton");

    const openModeField = document.getElementById("openModeField");

    const descriptionInput = document.querySelector("#description");

    const addEntryButton = document.querySelector("#addEntryButton");

    const cancelButton = document.querySelector("#cancelButton");

    const themeButton = document.querySelector("#themeButton");

    const entryRows = document.querySelector("#entryRows");

    const entryCount = document.querySelector("#entryCount");

    const listNotice = document.querySelector("#listNotice");

    const listWrap = document.querySelector("#listWrap");

    const toast = document.querySelector("#toast");

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

    let currentEditAppname = null;

    let modalOpener = null;

    // 当前表单弹窗锁定的类型（创建时由类型选择弹窗决定，编辑时取自条目）。
    // 类型值必须与 internal/model/entry.go 的 EntryType 常量一致。
    let formType = 'internal_port';

    const FORM_TYPE_LABELS = {
      internal_port: T.html.typeInternalPort,
      external_url: T.html.typeExternalUrl,
      folder: T.html.typeFolder,
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

      const isFolder = type === 'folder';

      document.querySelectorAll('.type-internal').forEach(el => el.classList.toggle('hidden', isExternal || isFolder));

      document.querySelectorAll('.type-external').forEach(el => el.classList.toggle('hidden', !isExternal));

      document.querySelectorAll('.type-folder').forEach(el => el.classList.toggle('hidden', !isFolder));

      // 文件夹类型固定 iframe：SDK 只在飞牛桌面宿主内可用，不给选打开方式。
      // 这两块用 .ghosted 占位而非 display:none：弹窗高度按最高页签渲染，切换页签才不会跳动。
      if (openModeField) openModeField.classList.toggle('ghosted', isFolder);

      // 桌面项用系统文件夹图标、不需要描述：图标与描述同属一行，folder 下整行占位隐藏。
      const iconDescRow = document.querySelector(".icon-desc-row");

      if (iconDescRow) iconDescRow.classList.toggle("ghosted", isFolder);

      urlInput.required = isExternal;

      portInput.required = !isExternal && !isFolder;

      if (folderPathInput) folderPathInput.required = isFolder;

      if (isFolder) updateFolderBrowseButton();

    }



    // 桌面宿主桥：懒加载 @trimjs/web-app，选目录器与桌面项生命周期共用。
    // SDK 只在飞牛桌面宿主内可用（独立浏览器 isStandaloneWeb=true），加载
    // 失败或环境不支持时按钮保持隐藏，路径手动输入始终可用。
    let hostAppState = "unloaded";

    let hostApp = null;

    // 并发调用共享同一次加载：否则后到的调用者会在 await import 期间看到
    // 中间态 "loading"，被调用点当成「不可用」而误报「仅桌面端可用」。
    let hostAppPromise = null;

    async function ensureHostApp() {

      if (!hostAppPromise) hostAppPromise = loadHostApp();

      return hostAppPromise;

    }

    async function loadHostApp() {

      hostAppState = "loading";

      try {

        const mod = await import(API_BASE + "/sdk/trim-web-app.js");

        const app = new mod.TrimApp();

        if (app.isStandaloneWeb) {

          hostAppState = "unavailable";

        } else {

          hostApp = app;

          hostAppState = "ready";

        }

      } catch (_) {

        hostAppState = "unavailable";

      }

      return hostAppState;

    }

    async function ensureFolderPicker() {

      return ensureHostApp();

    }

    const DESKTOP_REQ = {

      list: "appcgi.sac.entry.v1.getUserDesktop",

      add: "appcgi.sac.entry.v1.addUserDesktopEntry",

      del: "appcgi.sac.entry.v1.delUserDesktopEntry",

    };


    // 桌面收到这条 domainEvent 才会重拉图标列表。事件名与来源是桌面内部约定，
    // 对不上就只是不刷新，不会报错；事件失效时界面静默降级，下次操作刷新列表。
    const DESKTOP_EVENT = { entryChange: "user-desktop-entry-change", from: "trim-sac" };

    // 宿主桥代发的请求走桌面页自己那条 ?type=main 连接，连接对象要缓存，
    // 桌面对不到它就不会收到我们合成的广播。
    let hostMainSocket = null;

    let desktopNoticeTimer = null;

    async function captureHostMainSocket() {

      let proto = null;

      try {

        proto = window.top.WebSocket.prototype;

      } catch (_) {

        return null;

      }

      if (!proto || typeof proto.send !== "function") return null;

      const originalSend = proto.send;

      const tapped = [];

      const tap = function (data) {

        if (tapped.indexOf(this) < 0) tapped.push(this);

        return originalSend.apply(this, arguments);

      };

      try {

        proto.send = tap;

      } catch (_) {

        // 只读但可配置的属性只能用 defineProperty 覆盖。
        try {

          Object.defineProperty(proto, "send", { value: tap, writable: true, configurable: true });

        } catch (__) {

          return null;

        }

      }

      try {

        // 桥调用会被转成桌面连接上的一次 send，这就是抓到连接的手段。
        await hostApp.query({ req: DESKTOP_REQ.list, data: { language: "zh-CN" } });

      } catch (_) {

        // 抓到连接就够了，查询本身失败不影响补投。

      } finally {

        // 无论成败都要还原，绝不能让桌面长期跑在包装函数上。
        try {

          proto.send = originalSend;

        } catch (_) {

          try {

            Object.defineProperty(proto, "send", { value: originalSend, writable: true, configurable: true });

          } catch (__) {}

        }

        if (proto.send !== originalSend) console.warn("desktop socket prototype not restored");

      }

      return tapped.find((socket) => /type=main/.test(String(socket.url || ""))) || null;

    }

    async function injectDesktopEntryChange() {

      if ((await ensureHostApp()) !== "ready") return;

      // 桌面重连会换掉连接对象，旧对象的 readyState 变成关闭态，这里就会重新抓。
      if (!hostMainSocket || hostMainSocket.readyState !== 1) hostMainSocket = await captureHostMainSocket();

      if (!hostMainSocket || hostMainSocket.readyState !== 1) return;

      const frame = JSON.stringify({ domainEvent: { event: DESKTOP_EVENT.entryChange, from: DESKTOP_EVENT.from, body: "{}" } });

      try {

        if (typeof hostMainSocket.onmessage === "function") {

          hostMainSocket.onmessage({ data: frame });

        } else {

          // 桌面若改用 addEventListener 挂载，只能派发事件才能命中监听。
          hostMainSocket.dispatchEvent(new MessageEvent("message", { data: frame }));

        }

      } catch (_) {

        hostMainSocket = null;

      }

    }

    function notifyDesktopEntryChanged() {

      // 重建会先删后建，同一轮里只补投一次，避免连续两次重拉。
      if (desktopNoticeTimer) return;

      desktopNoticeTimer = setTimeout(() => {

        desktopNoticeTimer = null;

        // 补投是纯增强：拿不到连接或帧被宿主忽略时保持静默，不影响正常操作。
        injectDesktopEntryChange().catch(() => {});

      }, 300);

    }

    let desktopIndex = null;

    function hostErrorMessage(response) {

      if (!response) return T.dynamic.messages.desktopItemHostOnly;

      const parts = [];

      if (response.result) parts.push("result=" + response.result);

      if (response.errno) parts.push("errno=" + response.errno);

      if (response.errmsg) parts.push("errmsg=" + response.errmsg);

      return parts.join(" ").slice(0, 300) || T.dynamic.messages.desktopItemCreateFailed;

    }

    async function loadDesktopIndex() {

      if ((await ensureHostApp()) !== "ready") {

        desktopIndex = null;

        return null;

      }

      try {

        const response = await hostApp.query({ req: DESKTOP_REQ.list, data: { language: "zh-CN" } });

        if (!response || response.result !== "succ" || !response.data || !Array.isArray(response.data.list)) {

          desktopIndex = null;

          return null;

        }

        const index = new Map();

        for (const item of response.data.list) {

          if (!item || !item.entryKey) continue;

          index.set(item.entryKey, { name: item.name || "", folderPath: item.folderPath || "" });

        }

        desktopIndex = index;

        return index;

      } catch (error) {

        console.warn("load desktop items failed", error && error.message);

        desktopIndex = null;

        return null;

      }

    }

    function folderEntryStatus(entry) {

      if (entry.type !== "folder") return entry.status || "undeployed";

      if (!entry.desktopEntryKey) return "undeployed";

      // 索引拿不到时不臆测：按服务端记录显示，避免把"查询失败"显示成"已丢失"。
      if (desktopIndex && !desktopIndex.has(entry.desktopEntryKey)) return "desktop_missing";

      return "deployed";

    }

    async function saveDesktopEntryKey(appname, key) {

      const response = await fetchJsonWithTimeout(`${API_BASE}/api/entries/${encodeURIComponent(appname)}/desktop-item`, {

        method: "PUT",

        headers: { "Content-Type": "application/json" },

        body: JSON.stringify({ entryKey: key }),

        timeoutMs: 15000,

      });

      if (!response.ok) {

        const data = await response.json().catch(() => ({}));

        throw new Error(data.detail || data.error || "回写桌面项标识失败");

      }

      return response.json();

    }

    async function clearDesktopEntryKey(appname) {

      const response = await fetchJsonWithTimeout(`${API_BASE}/api/entries/${encodeURIComponent(appname)}/desktop-item`, {

        method: "DELETE",

        timeoutMs: 15000,

      });

      if (!response.ok) {

        const data = await response.json().catch(() => ({}));

        throw new Error(data.detail || data.error || "清空桌面项标识失败");

      }

      return response.json();

    }

    async function deleteDesktopItemByKey(key) {

      const response = await hostApp.query({ req: DESKTOP_REQ.del, data: { entryKey: key } });

      if (!response || response.result !== "succ") throw new Error(hostErrorMessage(response));

    }

    async function createDesktopItem(entry) {

      if ((await ensureHostApp()) !== "ready") throw new Error(T.dynamic.messages.desktopItemHostOnly);

      if (!desktopIndex) await loadDesktopIndex();

      // 索引不可用时不重复创建：宁可不动，也不在桌面留下两个同名项。
      if (entry.desktopEntryKey && (!desktopIndex || desktopIndex.has(entry.desktopEntryKey))) {

        return entry.desktopEntryKey;

      }

      if (desktopIndex) {

        // 服务端数据被清但桌面项还在时，按名称+路径认领，避免重复创建。
        for (const [key, item] of desktopIndex) {

          // 同名同路径的桌面项可能已经属于另一条条目：抢过来会让对方的图标凭空消失。
          const claimed = currentEntries.some((e) => e.appname !== entry.appname && e.desktopEntryKey === key);

          if (!claimed && item.name === entry.displayName && item.folderPath === entry.path) {

            await saveDesktopEntryKey(entry.appname, key);

            return key;

          }

        }

      }

      const response = await hostApp.query({

        req: DESKTOP_REQ.add,

        data: { itemType: "folder", name: entry.displayName, folderPath: entry.path, sort: 999999 },

      });

      if (!response || response.result !== "succ") throw new Error(hostErrorMessage(response));

      const key = response.data && response.data.entryKey;

      if (!key) throw new Error("宿主返回里没有 entryKey");

      try {

        await saveDesktopEntryKey(entry.appname, key);

      } catch (error) {

        let rollbackFailed = false;

        // 回写失败就把刚创建的项删掉，避免界面上看不见、桌面上删不掉的孤儿。
        await deleteDesktopItemByKey(key).catch((rollbackError) => {

          // 回滚也失败时桌面项会留下来，而界面已拿不到它的 key，只能留痕待手工清理。
          console.warn("desktop item rollback failed", rollbackError);

          rollbackFailed = true;

        });

        // 图标确实留在桌面，用户必须在失败提示里看到处理办法，不能只写日志。
        if (rollbackFailed) {

          const cleanupHint = formatText(T.dynamic.messages.desktopItemManualCleanup, { target: entry.displayName });

          if (error instanceof Error) {

            error.message = `${error.message} ${cleanupHint}`;

          } else {

            throw new Error(cleanupHint);

          }

        }

        throw error;

      }

      notifyDesktopEntryChanged();

      return key;

    }

    async function removeDesktopItem(entry) {

      if (!entry.desktopEntryKey) return;

      if ((await ensureHostApp()) !== "ready") throw new Error(T.dynamic.messages.desktopItemHostOnly);

      const response = await hostApp.query({ req: DESKTOP_REQ.del, data: { entryKey: entry.desktopEntryKey } });

      // 删除请求失败也可能已经删掉了；用实时列表确认，而不是只看 result。
      const index = await loadDesktopIndex();

      if (index && index.has(entry.desktopEntryKey)) throw new Error(hostErrorMessage(response));

      if (!index && (!response || response.result !== "succ")) throw new Error(hostErrorMessage(response));

      await clearDesktopEntryKey(entry.appname);

      notifyDesktopEntryChanged();

    }

    async function rebuildDesktopItem(entry) {

      // 旧项没确认消失前不能清 key：否则服务端记成「未创建」，桌面项却还在，
      // 再创建时名字/路径已变就认领不上，会留下新旧两个项。
      let removalConfirmed = !entry.desktopEntryKey;

      try {

        await removeDesktopItem(entry);

        removalConfirmed = true;

        // 移除已清服务端 key；沿用旧 key 会让创建命中「索引不可用视为已创建」的分支，
        // 结果什么都没建却提示已创建。
        await createDesktopItem({ ...entry, desktopEntryKey: null });

        await loadDesktopIndex();

        showNotice(formatText(T.dynamic.messages.desktopItemCreated, { target: entry.displayName }), "ok");

      } catch (error) {

        const failureMessage = removalConfirmed

          ? T.dynamic.messages.desktopItemCreateFailed

          : T.dynamic.messages.desktopItemRemoveFailed;

        if (removalConfirmed) {

          // 创建/回写失败时旧项已经不在了，残留的 key 只会让条目显示成「已创建」。
          await clearDesktopEntryKey(entry.appname).catch(() => {});

        }

        await loadDesktopIndex();

        showNotice(userMessage(ERROR_CODES.DEPLOY, failureMessage, errorDetail(error)), "warn");

      } finally {

        refreshListSilently();

      }

    }

    async function removeDesktopItemById(id, button) {

      const entry = currentEntries.find((item) => item.appname === id);

      if (!entry) return;

      if (button) button.disabled = true;

      showOperationOverlay(formatText(T.dynamic.operation.running, { name: T.dynamic.action.removeDesktopItem }));

      try {

        await removeDesktopItem(entry);

        await loadDesktopIndex();

        showNotice(formatText(T.dynamic.messages.desktopItemRemoved, { target: entry.displayName }), "ok");

        refreshListSilently();

      } catch (error) {

        showNotice(userMessage(ERROR_CODES.UNINSTALL, T.dynamic.messages.desktopItemRemoveFailed, errorDetail(error)), "warn");

      } finally {

        hideOperationOverlay();

        if (button) button.disabled = false;

      }

    }

    function updateFolderBrowseButton() {

      if (!folderBrowseButton) return;

      ensureFolderPicker().then((state) => {

        const ready = state === "ready";

        folderBrowseButton.classList.toggle("hidden", !ready);

        // 路径始终允许手动输入：手填路径在保存时经 root agent 校验存在性
        // （/api/validate-folder），不依赖宿主；「浏览」只是快捷填法。
        // 宿主不可用（独立浏览器）时按钮隐藏，但手填与校验不受影响。

      });

    }

    // 从 pickFile 的返回里提取路径。官方类型是 {code,msg,data:string[]}，
    // 但真机上主机返回的形状可能不同（嵌套对象、单项字符串等），
    // 这里按宽松顺序取第一个非空路径，取不到交给调用方报错。
    function extractPickedFolder(result) {

      if (!result) return "";

      let list = [];

      if (typeof result === "string") list = [result];

      else if (Array.isArray(result)) list = result;

      else if (Array.isArray(result.data)) list = result.data;

      else if (result.data && typeof result.data === "string") list = [result.data];

      for (const item of list) {

        if (typeof item === "string" && item.trim()) return item.trim();

        if (item && typeof item === "object") {

          for (const key of ["path", "filePath", "fullPath", "dir"]) {

            if (typeof item[key] === "string" && item[key].trim()) return item[key].trim();

          }

        }

      }

      return "";

    }

    async function browseFolderPath() {

      if (!hostApp || !folderPathInput) return;

      // 真机踩坑记录（pickUserFile 对第三方应用不可用）：
      // ① pickUserFile 确认后走主机授权登记，第三方应用实测必报「应用鉴权异常」
      //    （补全 api-scope、卸载重装均无效）；
      // ② pickUserFile 参数校验与文档不一致，缺 sidebarGroup 会报 10030。
      // 本应用只需要拿到路径字符串（openFileManager 官方明确不做权限校验，
      // 后端不读该目录），根本不需要授权登记，所以只调免授权的 pickFile。
      // 不用 pickUserFile 回退：它对第三方应用本就不可用，回退只会再弹一次选择器。
      // 选择器关闭/取消也走这里：返回空或「Operation failed」都没有路径，
      // 一律静默返回，不弹错误提示、不重试（快速关闭会被误判成“没弹窗”而重弹）。
      let result;

      try {

        result = await hostApp.pickFile({ directory: true });

      } catch (err) {

        // 选择器关闭/取消会 reject 一个空值，不是错误：静默返回，不弹提示。
        // 只有带明确信息的拒绝（方法不可用/参数被宿主拒绝）才提示。
        const detail = String(((err && err.message) || err || "")).trim();

        if (!detail) return;

        showGeneralToast(formatText(T.dynamic.errors.folderPickFailed, { detail: detail.slice(0, 120) }));

        return;

      }

      if (!result) return;

      const picked = extractPickedFolder(result);

      // 根目录是宿主的默认起始位置，不是用户的选择：选它等于没选，不填充。
      if (!picked || picked === "/") return;

      folderPathInput.value = picked;

      folderPathInput.classList.remove("invalid");

    }

    // 保存前校验手填的文件夹路径真实存在且是目录。存在性由 root agent 判定
    // （server 降权进程读不到 /vol1 共享目录，自己 stat 会误报）；agent 不可用
    // 返回 code="unavailable"，调用方 fail-closed，不放行可能不存在的路径。
    // 返回的是机器码 code，由调用方映射 i18n 文案——服务端的 reason 是人读
    // 英文，直接显示会出英文 toast。
    async function validateFolderPath(path) {

      try {

        // 服务端 stat-dir 上限 10s（statDirAgentTimeout），这里给 15s 让后端先返回，
        // 避免前端超时与后端各自判超时造成竞态（前端 10s 放弃、后端仍持有写锁）。
        const response = await fetchJsonWithTimeout(`${API_BASE}/api/validate-folder?path=${encodeURIComponent(path)}`, { timeoutMs: 15000 });

        const data = await response.json().catch(() => ({}));

        if (!response.ok) {

          return { valid: false, code: "unavailable" };

        }

        return { valid: !!data.valid, code: data.code || "not_found" };

      } catch (_) {

        return { valid: false, code: "unavailable" };

      }

    }

    // 把校验结果码映射成用户文案。
    function folderValidationMessage(code) {

      if (code === "bad_format") return T.dynamic.errors.folderBadFormat;

      if (code === "unavailable") return T.dynamic.errors.folderValidateUnavailable;

      return T.dynamic.errors.folderNotExist;

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

      if (folderPathInput) folderPathInput.value = "";

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

        if (folderPathInput) folderPathInput.value = entry.path || (entry.config && entry.config.path) || "";

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



    // 类型图标：与「新增配置」类型选择弹窗里的一致，列表里放在目标值前面，
    // 让端口号 / 链接 / 路径 一眼能分清类型，不必回去看名字猜。
    const TYPE_ICONS = {

      internal_port: '<rect x="2" y="3" width="20" height="7" rx="2"></rect><rect x="2" y="14" width="20" height="7" rx="2"></rect><path d="M6 6.5h.01M6 17.5h.01"></path>',

      external_url: '<path d="M10 13a5 5 0 0 0 7.54.54l3-3a5 5 0 0 0-7.07-7.07l-1.72 1.71"></path><path d="M14 11a5 5 0 0 0-7.54-.54l-3 3a5 5 0 0 0 7.07 7.07l1.71-1.71"></path>',

      folder: '<path d="M20 20a2 2 0 0 0 2-2V8a2 2 0 0 0-2-2h-7.9a2 2 0 0 1-1.69-.9L9.6 3.9A2 2 0 0 0 7.93 3H4a2 2 0 0 0-2 2v13a2 2 0 0 0 2 2Z"></path>',

    };

    function renderTargetCell(entry) {

      const target = entry.type === "external_url"
        ? (entry.url || "")
        : entry.type === "folder"
          ? (entry.path || "")
          : (entry.port || "");

      const icon = TYPE_ICONS[entry.type] || TYPE_ICONS.internal_port;

      return `<td class="entry-cell entry-target" title="${escapeHtml(target)}"><span class="target-value"><svg class="target-icon" viewBox="0 0 24 24" aria-hidden="true">${icon}</svg><span class="target-text">${escapeHtml(target)}</span></span></td>`;

    }

    function renderActions(entry) {

      const id = escapeHtml(entry.appname);

      if (entry.type === "folder") {

        const status = folderEntryStatus(entry);

        // 已知宿主不可用时把动作按钮置灰；首次渲染时状态还是 unloaded，
        // 按钮可用，点击后 createDesktopItem 会给出"请在桌面中打开"的提示。
        const disabled = hostAppState === "unavailable" ? " disabled" : "";

        const parts = [];

        if (status === "undeployed") {

          parts.push(`<button type="button" class="action-button primary"${disabled} data-deploy="${id}">${T.dynamic.action.createDesktopItem}</button>`);

        } else if (status === "desktop_missing") {

          parts.push(`<button type="button" class="action-button primary"${disabled} data-deploy="${id}">${T.dynamic.action.recreateDesktopItem}</button>`);

        } else {

          parts.push(`<button type="button" class="action-button ghost-danger"${disabled} data-remove-desktop="${id}">${T.dynamic.action.removeDesktopItem}</button>`);

        }

        parts.push(`<button type="button" class="action-button" data-edit="${id}">${T.dynamic.action.edit}</button>`);

        parts.push(`<button type="button" class="action-button ghost-danger" data-delete="${id}">${T.dynamic.action.delete}</button>`);

        return `<div class="entry-actions">${parts.join("")}</div>`;

      }

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

      desktop_missing: "warn",

    };

    function renderStatusDot(entry) {

      const isFolder = entry.type === "folder";

      const deployment = isFolder
        ? folderEntryStatus(entry)
        : (entry.deployment || entry.status || "undeployed");

      const label = isFolder
        ? (FOLDER_STATUS_LABELS[deployment] || deployment)
        : (DEPLOY_LABELS[deployment] || deployment);

      const tone = STATUS_DOT_TONES[deployment] || "danger";

      const runtime = isFolder ? "" : (entry.runtime || "");

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

          return `<tr>

            <td class="status-dot-cell">${renderStatusDot(entry)}</td>

            <td class="entry-cell"><span class="entry-title">${escapeHtml(entry.displayName)}</span></td>

            ${renderTargetCell(entry)}

            <td class="entry-cell col-center">${renderActions(entry)}</td>

          </tr>`;

        })

        .join("");

    }

    async function fetchEntries() {

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

      currentEntries = (data.entries || []).map((entry) => normalizeEntry(entry));

      renderEntries(currentEntries);

      if (currentEntries.some((entry) => entry.type === "folder")) {

        // 索引是异步的：先按服务端状态渲染，拿到桌面列表后再覆盖"丢失"状态。
        loadDesktopIndex().then(() => {

          if (currentEntries.some((entry) => entry.type === "folder")) renderEntries(currentEntries);

        });

      }

      return currentEntries;

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
      if (!shouldRefreshList()) return [];
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

      } else if (payload.type === 'folder') {

        const rawFolderPath = folderPathInput.value.trim();

        if (!rawFolderPath) {

          markInvalid(folderPathInput);

          showGeneralToast(T.dynamic.errors.requiredPath);

          return;

        }

        // 手填路径保存前校验真实存在且是目录：root agent 判定，fail-closed。
        // 「浏览」选出的路径天然存在，也会走这里，多一次确认无副作用。
        const folderCheck = await validateFolderPath(rawFolderPath);

        if (!folderCheck.valid) {

          markInvalid(folderPathInput);

          showGeneralToast(folderValidationMessage(folderCheck.code));

          return;

        }

        payload.path = rawFolderPath;

        payload.config = {

          path: rawFolderPath,

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

      const editSnapshot = currentEditAppname

        ? currentEntries.find((item) => item.appname === currentEditAppname)

        : null;

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

        const nextPath = payload.config && payload.config.path;

        const needsDesktopRebuild = editSnapshot

          && editSnapshot.type === "folder"

          && editSnapshot.desktopEntryKey

          && (editSnapshot.displayName !== payload.displayName || editSnapshot.path !== nextPath);

        if (needsDesktopRebuild) {

          // 重建带自己的提示与刷新：失败时若旧项已确认移除则 key 已清空、列表显示「未创建」，
          // 否则保留 key 让图标仍可被移除。
          rebuildDesktopItem({ ...editSnapshot, displayName: payload.displayName, path: nextPath }).catch(() => {});

        }

      } catch (error) {

        showGeneralToast(userMessage(ERROR_CODES.SAVE, T.dynamic.messages.saveFailed, errorDetail(error)));

      } finally {

        saveButton.disabled = false;

        saveButton.textContent = previousSaveLabel;

      }

    }

    async function deployEntry(id, button) {

      const targetEntry = currentEntries.find((item) => item.appname === id);

      if (targetEntry && targetEntry.type === "folder") {

        await deployFolderEntry(targetEntry, button);

        return;

      }

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

    async function deployFolderEntry(entry, button) {

      const noun = entry.desktopEntryKey ? T.dynamic.action.recreateDesktopItem : T.dynamic.action.createDesktopItem;

      if (button) button.disabled = true;

      showOperationOverlay(formatText(T.dynamic.operation.running, { name: noun }));

      try {

        if (entry.desktopEntryKey) await removeDesktopItem(entry);

        // 移除已清服务端 key；沿用旧 key 会让创建命中「索引不可用视为已创建」的分支，
        // 结果什么都没建却提示已创建。
        await createDesktopItem({ ...entry, desktopEntryKey: null });

        await loadDesktopIndex();

        showNotice(formatText(T.dynamic.messages.desktopItemCreated, { target: entry.displayName }), "ok");

        refreshListSilently();

      } catch (error) {

        // 移除阶段可能已经成功（服务端 key 已清），不重载界面会一直停在「已创建」。
        await loadDesktopIndex();

        refreshListSilently();

        showNotice(userMessage(ERROR_CODES.DEPLOY, T.dynamic.messages.desktopItemCreateFailed, errorDetail(error)), "warn");

      } finally {

        hideOperationOverlay();

        if (button) button.disabled = false;

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

        const entry = currentEntries.find((item) => item.appname === id);

        // 独立网页模式发不出宿主请求，桌面项删不掉，但定义照常删（设计 §9）；
        // 宿主可用却明确拒绝时仍然中止，避免留下孤儿图标。
        let desktopItemLeftBehind = false;

        if (entry && entry.type === "folder" && entry.desktopEntryKey) {

          if ((await ensureHostApp()) === "ready") {

            try {

              await removeDesktopItem(entry);

            } catch (error) {

              hideOperationOverlay();

              showGeneralToast(userMessage(ERROR_CODES.DELETE, T.dynamic.messages.desktopItemRemoveFailed, errorDetail(error)));

              if (button) button.disabled = false;

              return;

            }

          } else {

            desktopItemLeftBehind = true;

            console.warn("desktop item left behind: host unavailable", entry.appname, entry.desktopEntryKey);

          }

        }

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

        if (desktopItemLeftBehind) {

          // 提示必须合成一条：showToast 共用同一个节点，分两次弹会互相覆盖。
          const cleanupHint = formatText(T.dynamic.messages.desktopItemManualCleanup, { target: name });

          showNotice(formatText(T.dynamic.messages.deleted, { target: name }) + " " + cleanupHint, "warn");

        } else {

          showGeneralToast(formatText(T.dynamic.messages.deleted, { target: name }));

        }

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

    if (folderBrowseButton) folderBrowseButton.addEventListener("click", browseFolderPath);

    if (folderPathInput) folderPathInput.addEventListener("input", () => {

      // 创建时按路径末段建议唯一标识，可手改；编辑态输入框只读，不覆盖。

      if (folderPathInput.value !== "" && formAppnameInput && !formAppnameInput.readOnly && formAppnameInput.value === "") {

        const segment = folderPathInput.value.trim().split("/").filter(Boolean).pop() || "";

        const slug = segment.toLowerCase().replace(/[^a-z0-9-]/g, "-").replace(/^-+|-+$/g, "").slice(0, 12);

        if (slug && /^[a-z]/.test(slug)) formAppnameInput.value = slug;

      }

    });

    formAppnameInput.addEventListener("input", () => {

      if (formAppnameInput.readOnly) return;

      formAppnameInput.value = formAppnameInput.value.toLowerCase().replace(/[^a-z0-9-]/g, "");

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

      const removeDesktopId = target.getAttribute("data-remove-desktop");

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

      if (removeDesktopId) {

        await removeDesktopItemById(removeDesktopId, target);

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

