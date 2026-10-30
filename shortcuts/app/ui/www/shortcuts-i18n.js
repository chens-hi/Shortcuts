const SHORTCUTS_VERSION = "1.0.287";



const T = {

  html: {

    title: "快捷图标",

    subtitle: "管理端口快捷入口，生成独立桌面图标。",

    versionBadge: `v${SHORTCUTS_VERSION}`,

    themeTitle: "切换明暗主题",

    configList: "配置列表",

    addEntry: "新增配置",

    emptyList: "暂无配置，点击右上角\u201C新增配置\u201D创建第一条快捷入口。",

    loading: "加载中…",

    columnDisplayName: "名称",

    columnTarget: "目标",

    columnStatus: "状态",

    columnAction: "操作",

    typeInternalPort: "内置端口",

    typeExternalUrl: "外部链接",

    typeFolder: "文件夹",

    sectionDisplay: "捷径显示",

    appname: "唯一标识",

    appnameNote: "（创建后不可修改）",

    displayName: "捷径名字",

    port: "映射端口",

    protocol: "请求协议",

    protocolHttp: "http",

    protocolHttps: "https",

    path: "访问路径",

    urlLabel: "目标地址",

    folderPathLabel: "文件夹路径",

    folderPathPlaceholder: "例如 /vol1/1000/照片",

    folderBrowse: "浏览选择",

    urlPlaceholder: "请输入访问地址",

    openMode: "打开方式",

    openModeIframe: "桌面内嵌",

    openModeUrl: "新标签中",

    displayIcon: "捷径图标",

    defaultIconAlt: "默认图标",

    iconChoose: "点击选择",

    iconChooseTitle: "点击选择图标",

    iconClearTitle: "清除配置的图标，重置为默认的图标",

    iconRestoreTitle: "清除当前的图标，还原为配置的图标",

    iconCropTitle: "裁剪当前的图标",

    description: "描述介绍",

    descriptionPlaceholder: "留空使用默认应用介绍, 否则同步到应用描述中",

    cancel: "取消",

    save: "保存",

    formTitleCreate: "新增配置",

    formTitleEdit: "编辑配置",

    closeForm: "关闭配置窗口",

    typePickerTitle: "选择配置类型",

    typePickerClose: "关闭类型选择",

    typeDescInternal: "通过映射端口访问服务",

    typeDescExternal: "跳转到外部网址",

    typeDescFolder: "打开本地文件夹",

    cropTitle: "裁剪图标",

    closeCrop: "关闭裁剪",

    cropScale: "缩放",

    cropReset: "恢复默认",

    cropApply: "应用裁剪",

    cropNote: "拖动图片调整位置，滚轮或滑块调整大小。",

    operationProcessing: "处理中…",

    htmlLang: "zh-CN"

  },

  dynamic: {

    items: "{count} 项",

    iconPreviewAlt: "图标预览",

    reorderHandle: "拖拽排序",

    reorderAnnounce: "已移动到第 {position} 项",

    theme: {

      toDark: "切换到暗色主题",

      toLight: "切换到亮色主题"

    },

    status: {

      undeployed: "未部署",

      deployed: "已部署",

      outdated: "待更新",

      folderUndeployed: "未创建",

      folderDeployed: "已创建",

      desktopMissing: "桌面项丢失"

    },

    runtime: {

      running: "运行中",

      stopped: "已停止"

    },

    action: {

      deploy: "部署",

      update: "更新",

      uninstall: "卸载",

      disable: "停用",

      enable: "启用",

      edit: "编辑",

      delete: "删除",

      createDesktopItem: "创建",

      removeDesktopItem: "销毁",

      recreateDesktopItem: "重建",

      saving: "保存中…"

    },

    operation: {

      processing: "处理中…",

      running: "正在{name}，请稍候…"

    },

    errors: {

      readFallback: "读取配置失败，请稍后重试",

      refreshFallback: "刷新失败，请稍后重试",

      readListFallback: "读取失败，请稍后重试",

      imageFailed: "图片读取失败",

      requiredAppname: "请填写唯一标识",

      appnameInvalid: "唯一标识只能包含小写字母、数字和短横线",

      requiredDisplayName: "请填写捷径名字",

      displayNameMax: "捷径名字最多{max}个字符",

      requiredPort: "请填写映射端口",

      requiredUrl: "请填写跳转地址",

      requiredPath: "请填写文件夹路径",

      folderBadFormat: "文件夹路径必须以 / 开头，且不能是根目录",

      folderNotExist: "文件夹路径不存在或不是目录，请检查后重试",

      folderValidateUnavailable: "无法校验文件夹路径，请稍后重试",

      folderPickFailed: "选择文件夹失败：{detail}",

      deleteConfirm: "确定删除这条捷径？相关的桌面项或已部署子应用会一起清理"

    },

    messages: {

      errorCodeSuffix: "{message}（错误码：{code}）",

      iconFailed: "图标处理失败，请稍后重试",

      saveFailed: "保存配置失败，请稍后重试",

      reorderFailed: "保存顺序失败，已刷新列表",

      deployFailed: "{action}失败，请稍后重试",

      deployDisabled: "当前环境已禁用部署，请检查应用配置",

      deployNotEffective: "{action}已执行，但未确认到子应用，刷新后请确认",

      uninstallFailed: "卸载失败，请稍后重试",

      runtimeFailed: "{action}失败，请稍后重试",

      deleteFailed: "删除配置失败，请稍后重试",

      saveUpdated: "已更新 {target}",

      saveUnchanged: "配置无变化",

      saveCreated: "已新增 {target}",

      deleted: "已删除 {target}",

      deploySuccess: "已{name} {target}",

      deploySuccessStopped: "已{name} {target}，并保持停用",

      deploySkipped: "{target} 已是最新部署，无需重复部署",

      deleteCleanup: "{target} 已删除，子应用未移除",

      deleteUninstallPending: "{target} 已删除，子应用还在卸载，稍后可在应用中心确认",

      deployTimeout: "部署超时，应用中心未看到子应用，刷新后重试",

      runtimeStarted: "已启用 {name}",

      runtimeStopped: "已停用 {name}",

      desktopItemCreated: "已创建桌面项 {target}",

      desktopItemRemoved: "已销毁桌面项 {target}",

      desktopItemCreateFailed: "创建桌面项失败，请稍后重试",

      desktopItemRemoveFailed: "销毁桌面项失败，请稍后重试",

      desktopItemHostOnly: "请在飞牛桌面中打开「快捷图标」",

      desktopItemManualCleanup: "桌面图标「{target}」可能仍在，请手动删除"

    }

  }

};



const ERROR_CODES = {

  UNKNOWN: "SC-9999",

  READ: "SC-1001",

  IMAGE: "SC-1002",

  ICON: "SC-1003",

  SAVE: "SC-1004",

  DEPLOY: "SC-2001",

  DISABLED: "SC-2002",

  UNINSTALL: "SC-3001",

  RUNTIME: "SC-3002",

  DELETE: "SC-4001",

  DELETE_CLEANUP: "SC-4002",

  TIMEOUT: "SC-9001",

  VALIDATION: "SC-0001",

  NOT_FOUND: "SC-0002",

  METHOD: "SC-0003",

  RESOURCE: "SC-0004",

  SERVICE: "SC-5000"

};



const ERROR_MESSAGES = {

  [ERROR_CODES.READ]: "读取失败，请稍后重试",

  [ERROR_CODES.IMAGE]: "图片读取失败，请稍后重试",

  [ERROR_CODES.ICON]: "图标处理失败，请稍后重试",

  [ERROR_CODES.SAVE]: "保存失败，请稍后重试",

  [ERROR_CODES.DEPLOY]: "部署失败，请稍后重试",

  [ERROR_CODES.DISABLED]: "部署已禁用，请检查运行环境",

  [ERROR_CODES.UNINSTALL]: "卸载失败，请稍后重试",

  [ERROR_CODES.RUNTIME]: "操作失败，请稍后重试",

  [ERROR_CODES.DELETE]: "删除失败，请稍后重试",

  [ERROR_CODES.DELETE_CLEANUP]: "虽然配置已删除，但应用未卸载",

  [ERROR_CODES.TIMEOUT]: "操作超时，请稍后重试",

  [ERROR_CODES.UNKNOWN]: "操作失败，请稍后重试",

  [ERROR_CODES.VALIDATION]: "填写有误，请检查后重试",

  [ERROR_CODES.NOT_FOUND]: "配置不存在",

  [ERROR_CODES.METHOD]: "方法不允许",

  [ERROR_CODES.RESOURCE]: "资源不存在",

  [ERROR_CODES.SERVICE]: "服务异常，请稍后重试"

};



const SUCCESS_MESSAGES = {

  UNINSTALL_DONE: "已卸载 {target}",

  UNINSTALL_STOPPED_FALLBACK: "{target}：卸载命令不可用，已改为停用，要彻底移除请去应用中心",

  UNINSTALL_FAILED_MANUAL: "{target}：卸载和停用均未成功，请去应用中心处理",

  UNINSTALL_NOT_REMOVED: "{target}：卸载已执行，但安装目录还在，请刷新后确认"

};



function formatText(template, vars = {}) {

  return String(template || "").replace(/\{(\w+)\}/g, (match, key) => vars[key] ?? match);

}



function initLocalizedHtml() {

  document.documentElement.lang = T.html.htmlLang;

  document.title = T.html.title;



  const setText = (id, value) => {

    const element = document.getElementById(id);

    if (element) element.textContent = value;

  };



  const themeButton = document.getElementById("themeButton");

  if (themeButton) {

    themeButton.title = T.html.themeTitle;

    themeButton.setAttribute("aria-label", T.html.themeTitle);

  }

  setText("addEntryButton", T.html.addEntry);

  setText("listNotice", T.html.emptyList);

  setText("listLoading", T.html.loading);

  setText("cancelButton", T.html.cancel);

  setText("saveButton", T.html.save);

  setText("entryFormTitle", T.html.formTitleCreate);

  const entryFormCloseButton = document.getElementById("entryFormCloseButton");

  if (entryFormCloseButton) entryFormCloseButton.setAttribute("aria-label", T.html.closeForm);

  setText("cropTitle", T.html.cropTitle);

  setText("cropResetButton", T.html.cropReset);

  setText("cropApplyButton", T.html.cropApply);

  setText("operationMessage", T.html.operationProcessing);

  setText("cropScaleLabel", T.html.cropScale);

  setText("cropNote", T.html.cropNote);

  setText("configListTitle", T.html.configList);

  setText("typeInternalPort", T.html.typeInternalPort);

  setText("typeExternalURL", T.html.typeExternalUrl);

  setText("typeFolderLabel", T.html.typeFolder);

  setText("typePickerTitle", T.html.typePickerTitle);

  const typePickerCloseButton = document.getElementById("typePickerCloseButton");

  if (typePickerCloseButton) typePickerCloseButton.setAttribute("aria-label", T.html.typePickerClose);

  setText("typeCardInternal", T.html.typeInternalPort);

  setText("typeCardExternal", T.html.typeExternalUrl);

  setText("typeCardFolder", T.html.typeFolder);

  setText("typeCardInternalDesc", T.html.typeDescInternal);

  setText("typeCardExternalDesc", T.html.typeDescExternal);

  setText("typeCardFolderDesc", T.html.typeDescFolder);

  setText("folderPathLabel", T.html.folderPathLabel);

  setText("folderBrowseButton", T.html.folderBrowse);

  setText("urlLabel", T.html.urlLabel);

  setText("appnameLabelText", T.html.appname);

  setText("appnameNote", T.html.appnameNote);

  const _urlInput = document.querySelector("#url");

  if (_urlInput) _urlInput.placeholder = T.html.urlPlaceholder;

  const _folderPathInput = document.querySelector("#folderPath");

  if (_folderPathInput) _folderPathInput.placeholder = T.html.folderPathPlaceholder;



  const closeCropButton = document.getElementById("cropCloseButton");

  if (closeCropButton) closeCropButton.setAttribute("aria-label", T.html.closeCrop);



  const subtitle = document.querySelector(".subtitle");

  if (subtitle) subtitle.textContent = T.html.subtitle;

  const versionBadge = document.querySelector(".badge");

  if (versionBadge) versionBadge.textContent = T.html.versionBadge;



  const columns = document.querySelectorAll(".entry-table th");

  if (columns.length >= 5) {

    // 前两列是拖拽手柄和状态圆点，窄列放不下文字，只给读屏用的列名。
    columns[0].setAttribute("aria-label", T.dynamic.reorderHandle);

    columns[1].setAttribute("aria-label", T.html.columnStatus);

    columns[2].textContent = T.html.columnDisplayName;

    columns[3].textContent = T.html.columnTarget;

    columns[4].textContent = T.html.columnAction;

  }



  const labelById = {

    displayNameLabel: T.html.displayName,

    portLabel: T.html.port,

    protocolLabel: T.html.protocol,

    pathLabel: T.html.path,

    urlLabel: T.html.urlLabel,

    openModeLabel: T.html.openMode,

    descriptionLabel: T.html.description,

    displayIconLabel: T.html.displayIcon

  };

  for (const [id, text] of Object.entries(labelById)) {

    const label = document.getElementById(id);

    if (label) label.textContent = text;

  }

  const defaultIconImage = document.querySelector("#iconPreview img");

  if (defaultIconImage) defaultIconImage.setAttribute("alt", T.html.defaultIconAlt);

  const iconPreviewTrigger = document.getElementById("iconPreview");

  if (iconPreviewTrigger) iconPreviewTrigger.title = T.html.iconChooseTitle;

  setText("iconPreviewOverlayHint", T.html.iconChoose);

  const iconButtonTitles = {

    iconClearButton: T.html.iconClearTitle,

    iconRestoreButton: T.html.iconRestoreTitle,

    iconCropButton: T.html.iconCropTitle,

  };

  for (const [id, text] of Object.entries(iconButtonTitles)) {

    const button = document.getElementById(id);

    if (!button) continue;

    button.title = text;

    button.setAttribute("aria-label", text);

  }



  const protocolOptions = document.querySelectorAll('#protocol option');

  if (protocolOptions.length >= 2) {

    protocolOptions[0].textContent = T.html.protocolHttp;

    protocolOptions[1].textContent = T.html.protocolHttps;

  }



  const openModeOptions = document.querySelectorAll('#openMode option');

  if (openModeOptions.length >= 2) {

    openModeOptions[0].textContent = T.html.openModeIframe;

    openModeOptions[1].textContent = T.html.openModeUrl;

  }



  const descriptionInput = document.querySelector("#description");

  if (descriptionInput) descriptionInput.placeholder = T.html.descriptionPlaceholder;

}