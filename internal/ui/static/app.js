(() => {
  const ensureRoot = () => {
    let root = document.getElementById("appToastRoot");
    if (root) return root;

    root = document.createElement("div");
    root.id = "appToastRoot";
    root.className = "toast-root";
    document.body.appendChild(root);
    return root;
  };

  const buildQRCodeURL = (value, size = 240) => {
    if (!value) return "";
    const params = new URLSearchParams({
      data: value,
      size: String(size),
    });
    return `/qr.png?${params.toString()}`;
  };

  const hydrateQRCodeImages = (root = document) => {
    root.querySelectorAll("[data-qr-image]").forEach((image) => {
      const value = image.dataset.qrValue;
      if (!value) return;
      const size = Number.parseInt(image.dataset.qrSize || "240", 10) || 240;
      image.src = buildQRCodeURL(value, size);
      if (!image.alt) {
        image.alt = image.dataset.qrAlt || "分享二维码";
      }
    });
  };

  const openQRCodeDialog = ({
    title = "扫码分享",
    description = "使用手机扫码打开这个分享链接。",
    label = "分享链接",
    value,
    password = "",
  } = {}) => {
    if (!value) return;

    document.getElementById("appQrDialog")?.remove();

    const modal = document.createElement("div");
    modal.className = "modal-backdrop";
    modal.id = "appQrDialog";

    const card = document.createElement("section");
    card.className = "modal-card qr-dialog";

    const head = document.createElement("div");
    head.className = "qr-dialog-head";

    const headCopy = document.createElement("div");
    const heading = document.createElement("h3");
    heading.textContent = title;
    const desc = document.createElement("p");
    desc.textContent = description;
    headCopy.append(heading, desc);

    const closeButton = document.createElement("button");
    closeButton.type = "button";
    closeButton.className = "ghost-btn compact-btn";
    closeButton.setAttribute("data-close-modal", "");
    closeButton.textContent = "关闭";
    head.append(headCopy, closeButton);

    const body = document.createElement("div");
    body.className = "qr-dialog-body";

    const preview = document.createElement("div");
    preview.className = "qr-preview-card";
    const image = document.createElement("img");
    image.className = "qr-preview-image";
    image.setAttribute("data-qr-image", "");
    image.dataset.qrValue = value;
    image.dataset.qrSize = "320";
    image.alt = `${title}二维码`;
    image.referrerPolicy = "no-referrer";
    preview.appendChild(image);

    const meta = document.createElement("div");
    meta.className = "qr-meta-card";

    const metaLabel = document.createElement("span");
    metaLabel.className = "qr-meta-label";
    metaLabel.textContent = label;

    const code = document.createElement("code");
    code.textContent = value;

    const note = document.createElement("p");
    note.className = "qr-meta-note";
    note.textContent = "支持手机扫码打开，也可以复制后直接转发。";

    const metaContent = document.createElement("div");
    metaContent.className = "qr-meta-group";
    metaContent.append(metaLabel, code);

    meta.append(metaContent);

    if (password) {
      const passwordContent = document.createElement("div");
      passwordContent.className = "qr-meta-group";

      const passwordLabel = document.createElement("span");
      passwordLabel.className = "qr-meta-label";
      passwordLabel.textContent = "访问密码";

      const passwordCode = document.createElement("code");
      passwordCode.textContent = password;

      const passwordActions = document.createElement("div");
      passwordActions.className = "button-row";

      const copyPasswordButton = document.createElement("button");
      copyPasswordButton.type = "button";
      copyPasswordButton.className = "ghost-btn compact-btn";
      copyPasswordButton.textContent = "复制密码";
      copyPasswordButton.addEventListener("click", () => {
        window.AppUI.copyText(password, "已复制访问密码");
      });

      passwordActions.appendChild(copyPasswordButton);
      passwordContent.append(passwordLabel, passwordCode, passwordActions);
      meta.append(passwordContent);
    }

    const actions = document.createElement("div");
    actions.className = "button-row";

    const copyButton = document.createElement("button");
    copyButton.type = "button";
    copyButton.className = "ghost-btn compact-btn";
    copyButton.textContent = "复制链接";
    copyButton.addEventListener("click", () => {
      window.AppUI.copyText(value, "已复制分享链接");
    });

    const openLink = document.createElement("a");
    openLink.className = "primary-btn compact-btn";
    openLink.href = value;
    openLink.target = "_blank";
    openLink.rel = "noreferrer";
    openLink.textContent = "打开链接";

    actions.append(copyButton, openLink);
    meta.append(note, actions);
    body.append(preview, meta);
    card.append(head, body);
    modal.appendChild(card);
    document.body.appendChild(modal);

    const close = () => {
      document.removeEventListener("keydown", onKeyDown);
      modal.remove();
    };
    const onKeyDown = (event) => {
      if (event.key === "Escape") {
        close();
      }
    };

    document.addEventListener("keydown", onKeyDown);
    modal.addEventListener("click", (event) => {
      if (event.target === modal) {
        close();
      }
    });
    closeButton.addEventListener("click", close);

    hydrateQRCodeImages(modal);
  };

  const inferType = (message, fallback = "info") => {
    if (!message) return fallback;
    if (/(失败|错误|拒绝|用尽|过期|无法)/.test(message)) return "error";
    if (/(成功|完成|已|复制|保存)/.test(message)) return "success";
    return fallback;
  };

  const notify = ({ message, type = "info", duration = 2200 } = {}) => {
    if (!message) return;

    const root = ensureRoot();
    const item = document.createElement("div");
    const titleMap = {
      success: "操作成功",
      error: "操作失败",
      info: "操作提示",
    };
    item.className = `toast-item toast-${type}`;
    item.innerHTML = `
      <div class="toast-title">${titleMap[type] || titleMap.info}</div>
      <div class="toast-message"></div>
    `;
    item.querySelector(".toast-message").textContent = message;
    root.appendChild(item);

    window.requestAnimationFrame(() => {
      item.classList.add("toast-visible");
    });

    const remove = () => {
      item.classList.remove("toast-visible");
      window.setTimeout(() => item.remove(), 180);
    };

    window.setTimeout(remove, duration);
    item.addEventListener("click", remove, { once: true });
  };

  const legacyCopyText = (value) => {
    const selection = document.getSelection();
    const savedRanges = [];
    if (selection) {
      for (let index = 0; index < selection.rangeCount; index += 1) {
        savedRanges.push(selection.getRangeAt(index).cloneRange());
      }
    }

    const activeElement = document.activeElement instanceof HTMLElement ? document.activeElement : null;
    const area = document.createElement("textarea");
    area.value = value;
    area.setAttribute("aria-hidden", "true");
    area.style.position = "fixed";
    area.style.top = "0";
    area.style.left = "-9999px";
    area.style.opacity = "0";
    area.style.pointerEvents = "none";

    document.body.appendChild(area);
    area.focus({ preventScroll: true });
    area.select();
    area.setSelectionRange(0, area.value.length);

    let copied = false;
    try {
      copied = document.execCommand("copy");
    } finally {
      document.body.removeChild(area);

      if (selection) {
        selection.removeAllRanges();
        savedRanges.forEach((range) => selection.addRange(range));
      }

      if (activeElement) {
        activeElement.focus({ preventScroll: true });
      }
    }

    return copied;
  };

  const copyText = async (value, successMessage = "复制成功", options = {}) => {
    if (!value) return false;
    const {
      silentFailure = false,
      silentSuccess = false,
      requireUserGesture = false,
      failureMessage = "复制失败，请手动复制。",
    } = options;

    if (requireUserGesture && !(navigator.userActivation && navigator.userActivation.isActive)) {
      if (!silentFailure) {
        notify({ message: failureMessage, type: "error" });
      }
      return false;
    }

    try {
      await navigator.clipboard.writeText(value);
      if (!silentSuccess) {
        notify({ message: successMessage, type: "success" });
      }
      return true;
    } catch (_) {
      try {
        const copied = legacyCopyText(value);
        if (!copied) {
          throw new Error("copy failed");
        }
        if (!silentSuccess) {
          notify({ message: successMessage, type: "success" });
        }
        return true;
      } catch (_) {
        if (!silentFailure) {
          notify({ message: failureMessage, type: "error" });
        }
        return false;
      }
    }
  };

  const notifyFromDataset = (element) => {
    if (!element) return;
    const message = element.dataset.pageNotice;
    if (!message) return;
    notify({
      message,
      type: element.dataset.pageNoticeType || inferType(message),
      duration: 2400,
    });
  };

  const bindCopyButtons = (root = document) => {
    root.querySelectorAll(".js-copy").forEach((button) => {
      if (button.dataset.copyBound === "true") return;
      button.dataset.copyBound = "true";
      button.addEventListener("click", () => {
        const message = button.dataset.copyMessage || "复制成功";
        window.AppUI.copyText(button.dataset.copyValue, message);
      });
    });
  };

  const bindShareModal = (modal) => {
    if (!modal || modal.dataset.modalBound === "true") return;
    modal.dataset.modalBound = "true";
    modal.addEventListener("click", (event) => {
      if (event.target === modal) {
        modal.remove();
      }
    });
    modal.querySelectorAll("[data-close-modal]").forEach((button) => {
      button.addEventListener("click", () => modal.remove());
    });
    bindCopyButtons(modal);
    hydrateQRCodeImages(modal);
  };

  const bindClosableModal = (modal) => {
    if (!modal || modal.dataset.modalBound === "true") return;
    modal.dataset.modalBound = "true";
    const close = () => {
      document.removeEventListener("keydown", onKeyDown);
      modal.remove();
    };
    const onKeyDown = (event) => {
      if (event.key === "Escape") {
        close();
      }
    };

    document.addEventListener("keydown", onKeyDown);
    modal.addEventListener("click", (event) => {
      if (event.target === modal) {
        close();
      }
    });
    modal.querySelectorAll("[data-close-modal]").forEach((button) => {
      button.addEventListener("click", close);
    });
  };

  const toggleRemainingInputState = (form) => {
    const policySelect = form.querySelector('[name="download_policy"]');
    const remainingInput = form.querySelector('[name="remaining_downloads"]');
    if (!policySelect || !remainingInput) return;

    const isLimited = policySelect.value === "limited";
    remainingInput.disabled = !isLimited;
    remainingInput.closest("label")?.classList.toggle("disabled-field", !isLimited);
  };

  const openShareDetailDialog = (trigger) => {
    const shareURL = trigger?.dataset?.shareUrl || "";
    if (!shareURL) return;

    document.getElementById("appShareDetailDialog")?.remove();

    const modal = document.createElement("div");
    modal.className = "modal-backdrop";
    modal.id = "appShareDetailDialog";

    const card = document.createElement("section");
    card.className = "modal-card share-detail-dialog";

    const head = document.createElement("div");
    head.className = "qr-dialog-head";

    const headCopy = document.createElement("div");
    const heading = document.createElement("h3");
    heading.textContent = trigger.dataset.shareName || "分享详情";
    const desc = document.createElement("p");
    desc.textContent = "复制链接、查看二维码和密码信息都集中在这里，避免列表内重复堆叠。";
    headCopy.append(heading, desc);

    const closeButton = document.createElement("button");
    closeButton.type = "button";
    closeButton.className = "ghost-btn compact-btn";
    closeButton.setAttribute("data-close-modal", "");
    closeButton.textContent = "关闭";
    head.append(headCopy, closeButton);

    const grid = document.createElement("div");
    grid.className = "share-detail-grid";

    const createItem = (label, value, copyLabel, qrDescription, password = "") => {
      if (!value) return null;
      const item = document.createElement("div");
      item.className = "share-result-item";

      const title = document.createElement("span");
      title.textContent = label;
      const code = document.createElement("code");
      code.textContent = value;

      const actions = document.createElement("div");
      actions.className = "button-row";

      const copyButton = document.createElement("button");
      copyButton.type = "button";
      copyButton.className = "ghost-btn compact-btn js-copy";
      copyButton.dataset.copyValue = value;
      copyButton.dataset.copyMessage = copyLabel;
      copyButton.textContent = "复制";

      const qrButton = document.createElement("button");
      qrButton.type = "button";
      qrButton.className = "ghost-btn compact-btn";
      qrButton.dataset.openQr = "";
      qrButton.dataset.qrValue = value;
      qrButton.dataset.qrTitle = "扫码分享";
      qrButton.dataset.qrDescription = qrDescription;
      qrButton.dataset.qrLabel = label;
      if (password) {
        qrButton.dataset.qrPassword = password;
      }
      qrButton.textContent = "二维码";

      actions.append(copyButton, qrButton);
      item.append(title, code, actions);
      return item;
    };

    const primary = createItem("普通链接", shareURL, "已复制普通分享链接", "使用手机扫码即可打开当前分享链接。", trigger.dataset.sharePassword || "");
    const passwordURL = createItem("带密码链接", trigger.dataset.sharePasswordUrl || "", "已复制带密码分享链接", "手机扫码后可直接打开带密码的分享链接。", trigger.dataset.sharePassword || "");
    const directURL = createItem("快捷直达链接", trigger.dataset.shareDirectUrl || "", "已复制快捷直达链接", "手机扫码后可直接打开快捷直达链接。");
    const passwordText = createItem("分享密码", trigger.dataset.sharePassword || "", "已复制分享密码", "");

    [primary, passwordURL, directURL, passwordText].forEach((item) => {
      if (item) {
        grid.appendChild(item);
      }
    });

    card.append(head, grid);
    modal.appendChild(card);
    document.body.appendChild(modal);
    bindCopyButtons(modal);
    bindClosableModal(modal);
  };

  const openShareEditDialog = (trigger) => {
    if (!trigger?.dataset?.shareId) return;

    document.getElementById("appShareEditDialog")?.remove();

    const modal = document.createElement("div");
    modal.className = "modal-backdrop";
    modal.id = "appShareEditDialog";

    const card = document.createElement("section");
    card.className = "modal-card share-edit-dialog";

    const head = document.createElement("div");
    head.className = "qr-dialog-head";

    const headCopy = document.createElement("div");
    const heading = document.createElement("h3");
    heading.textContent = "编辑分享";
    const desc = document.createElement("p");
    desc.textContent = "在弹窗中调整过期时间和剩余下载次数，避免列表内表单占空间。";
    headCopy.append(heading, desc);

    const closeButton = document.createElement("button");
    closeButton.type = "button";
    closeButton.className = "ghost-btn compact-btn";
    closeButton.setAttribute("data-close-modal", "");
    closeButton.textContent = "关闭";
    head.append(headCopy, closeButton);

    const meta = document.createElement("div");
    meta.className = "share-edit-meta";

    const nameChip = document.createElement("span");
    nameChip.className = "status-chip";
    nameChip.textContent = trigger.dataset.shareName || "未命名资源";

    const typeChip = document.createElement("span");
    typeChip.className = "status-chip";
    typeChip.textContent = (trigger.dataset.shareKind || "").toUpperCase() || "SHARE";

    const codeChip = document.createElement("span");
    codeChip.className = "status-chip";
    codeChip.textContent = trigger.dataset.shareCode || "";

    meta.append(nameChip, typeChip, codeChip);

    const form = document.createElement("form");
    form.method = "post";
    form.action = `/admin/shares/${trigger.dataset.shareId}/update`;
    form.className = "share-edit-form-modal";

    const redirectInput = document.createElement("input");
    redirectInput.type = "hidden";
    redirectInput.name = "redirect_to";
    redirectInput.value = trigger.dataset.shareRedirect || "/admin/shares";
    form.appendChild(redirectInput);

    const expireLabel = document.createElement("label");
    const expireSpan = document.createElement("span");
    expireSpan.textContent = "过期时间";
    const expireSelect = document.createElement("select");
    expireSelect.name = "expire_option";

    const expireOptions = [
      ["keep_current", trigger.dataset.shareExpireLabel || "保持当前"],
      ["expired_now", "立即过期"],
      ["7h", "7小时"],
      ["6h", "6小时"],
      ["24h", "24小时"],
      ["7d", "7天"],
      ["30d", "30天"],
      ["365d", "365天"],
      ["never", "永不过期"],
    ];
    expireOptions.forEach(([value, label]) => {
      const option = document.createElement("option");
      option.value = value;
      option.textContent = label;
      option.selected = value === (trigger.dataset.shareExpire || "7d");
      expireSelect.appendChild(option);
    });
    expireLabel.append(expireSpan, expireSelect);

    const policyLabel = document.createElement("label");
    const policySpan = document.createElement("span");
    policySpan.textContent = "下载策略";
    const policySelect = document.createElement("select");
    policySelect.name = "download_policy";
    [
      ["unlimited", "不限次"],
      ["limited", "限剩余次数"],
    ].forEach(([value, label]) => {
      const option = document.createElement("option");
      option.value = value;
      option.textContent = label;
      option.selected = value === (trigger.dataset.shareDownloadPolicy || "unlimited");
      policySelect.appendChild(option);
    });
    policyLabel.append(policySpan, policySelect);

    const remainingLabel = document.createElement("label");
    const remainingSpan = document.createElement("span");
    remainingSpan.textContent = "剩余下载次数";
    const remainingInput = document.createElement("input");
    remainingInput.type = "number";
    remainingInput.name = "remaining_downloads";
    remainingInput.min = "0";
    remainingInput.placeholder = "0 表示立即耗尽";
    remainingInput.value = trigger.dataset.shareRemainingDownloads || "0";
    remainingLabel.append(remainingSpan, remainingInput);

    const actions = document.createElement("div");
    actions.className = "share-edit-actions";

    const cancelButton = document.createElement("button");
    cancelButton.type = "button";
    cancelButton.className = "ghost-btn compact-btn";
    cancelButton.setAttribute("data-close-modal", "");
    cancelButton.textContent = "取消";

    const submitButton = document.createElement("button");
    submitButton.type = "submit";
    submitButton.className = "primary-btn compact-btn";
    submitButton.textContent = "保存修改";

    actions.append(cancelButton, submitButton);
    form.append(expireLabel, policyLabel, remainingLabel, actions);
    card.append(head, meta, form);
    modal.appendChild(card);
    document.body.appendChild(modal);

    policySelect.addEventListener("change", () => toggleRemainingInputState(form));
    toggleRemainingInputState(form);
    bindClosableModal(modal);
  };

  const initializePasswordButtons = () => {
    document.querySelectorAll("[data-generate-password]").forEach((button) => {
      button.addEventListener("click", () => {
        const input = button.parentElement.querySelector("[data-password-input]");
        if (!input) return;
        const chars = "ABCDEFGHJKLMNPQRSTUVWXYZabcdefghijkmnopqrstuvwxyz23456789";
        const array = new Uint32Array(8);
        crypto.getRandomValues(array);
        input.value = Array.from(array, (n) => chars[n % chars.length]).join("");
        input.focus();
        window.AppUI.copyText(input.value, "随机密码已生成并复制");
      });
    });
  };

  const initializeBatchSelection = () => {
    const checkboxes = Array.from(document.querySelectorAll(".item-checkbox"));
    const selectedCount = document.getElementById("selectedCount");
    const selectAll = document.getElementById("selectAllItems");
    if (checkboxes.length === 0) return;

    const updateSelection = () => {
      const checked = checkboxes.filter((input) => input.checked);
      if (selectedCount) {
        selectedCount.textContent = String(checked.length);
      }
      if (selectAll) {
        selectAll.checked = checked.length > 0 && checked.length === checkboxes.length;
        selectAll.indeterminate = checked.length > 0 && checked.length < checkboxes.length;
      }
    };

    checkboxes.forEach((checkbox) => {
      checkbox.addEventListener("change", updateSelection);
    });

    if (selectAll) {
      selectAll.addEventListener("change", () => {
        checkboxes.forEach((checkbox) => {
          checkbox.checked = selectAll.checked;
        });
        updateSelection();
      });
    }

    const copySelectedBtn = document.getElementById("copySelectedBtn");
    if (copySelectedBtn) {
      copySelectedBtn.addEventListener("click", () => {
        const values = checkboxes
          .filter((checkbox) => checkbox.checked)
          .map((checkbox) => checkbox.dataset.copyValue)
          .filter(Boolean);
        if (values.length === 0) {
          window.AppUI.notify({ message: "请先选择要复制的资源", type: "info" });
          return;
        }
        window.AppUI.copyText(values.join("\n"), "已复制选中分享链接");
      });
    }

    updateSelection();
  };

  const initializeUploadForms = () => {
    document.querySelectorAll("[data-upload-form]").forEach((uploadForm) => {
      if (!(window.XMLHttpRequest && window.FormData)) return;

      const progressWrap = uploadForm.querySelector("[data-upload-progress]");
      const progressBar = uploadForm.querySelector("[data-upload-bar]");
      const progressPercent = uploadForm.querySelector("[data-upload-percent]");
      const progressStatus = uploadForm.querySelector("[data-upload-status]");
      const submitButton = uploadForm.querySelector("[data-upload-submit]");

      const setUploadState = ({ visible, percent, status, disabled, indeterminate }) => {
        if (progressWrap) {
          progressWrap.hidden = !visible;
          progressWrap.classList.toggle("upload-progress-indeterminate", Boolean(indeterminate));
        }
        if (progressBar && typeof percent === "number") {
          progressBar.style.width = `${Math.max(0, Math.min(100, percent))}%`;
        }
        if (progressPercent && typeof percent === "number") {
          progressPercent.textContent = `${Math.round(percent)}%`;
        }
        if (progressStatus && status) {
          progressStatus.textContent = status;
        }
        if (submitButton) {
          submitButton.disabled = Boolean(disabled);
          submitButton.textContent = disabled ? "正在上传..." : "上传并生成分享";
        }
      };

      uploadForm.addEventListener("submit", (event) => {
        event.preventDefault();

        const fileInput = uploadForm.querySelector("[data-drop-input]");
        if (fileInput && (!fileInput.files || fileInput.files.length === 0)) {
          window.AppUI.notify({ message: "请先选择要上传的文件", type: "info" });
          return;
        }

        const xhr = new XMLHttpRequest();
        xhr.open(uploadForm.method || "POST", uploadForm.action);
        xhr.responseType = "json";
        xhr.setRequestHeader("Accept", "application/json");
        xhr.setRequestHeader("X-Requested-With", "XMLHttpRequest");

        setUploadState({
          visible: true,
          percent: 0,
          status: "准备上传文件",
          disabled: true,
          indeterminate: false,
        });

        xhr.upload.addEventListener("progress", (progressEvent) => {
          if (progressEvent.lengthComputable && progressEvent.total > 0) {
            const percent = (progressEvent.loaded / progressEvent.total) * 100;
            setUploadState({
              visible: true,
              percent,
              status: percent >= 100 ? "文件已上传，正在生成分享链接" : "正在上传文件",
              disabled: true,
              indeterminate: false,
            });
            return;
          }
          setUploadState({
            visible: true,
            percent: 35,
            status: "正在上传文件",
            disabled: true,
            indeterminate: true,
          });
        });

        xhr.addEventListener("load", () => {
          const result = xhr.response;
          if (xhr.status >= 200 && xhr.status < 300 && result && result.ok) {
            setUploadState({
              visible: true,
              percent: 100,
              status: "上传完成，正在刷新列表",
              disabled: true,
              indeterminate: false,
            });
            window.location.assign(result.redirect || uploadForm.dataset.redirectTarget || "/admin/files");
            return;
          }

          setUploadState({
            visible: false,
            percent: 0,
            status: "准备上传文件",
            disabled: false,
            indeterminate: false,
          });
          window.AppUI.notify({
            message: result?.message || "上传失败，请稍后重试。",
            type: "error",
          });
        });

        xhr.addEventListener("error", () => {
          setUploadState({
            visible: false,
            percent: 0,
            status: "准备上传文件",
            disabled: false,
            indeterminate: false,
          });
          window.AppUI.notify({ message: "上传失败，请检查网络后重试。", type: "error" });
        });

        xhr.send(new FormData(uploadForm));
      });
    });
  };

  const initializeDropzones = () => {
    document.querySelectorAll("[data-dropzone]").forEach((dropzone) => {
      const input = dropzone.querySelector("[data-drop-input]");
      const box = dropzone.querySelector(".dropzone-box");
      if (!input || !box) return;

      box.addEventListener("click", () => {
        input.click();
      });
      ["dragenter", "dragover"].forEach((eventName) => {
        dropzone.addEventListener(eventName, (event) => {
          event.preventDefault();
          dropzone.classList.add("dropzone-active");
        });
      });
      ["dragleave", "dragend", "drop"].forEach((eventName) => {
        dropzone.addEventListener(eventName, (event) => {
          event.preventDefault();
          dropzone.classList.remove("dropzone-active");
        });
      });
      dropzone.addEventListener("drop", (event) => {
        const files = event.dataTransfer?.files;
        if (!files || files.length === 0) return;
        input.files = files;
        const firstFile = files[0];
        if (firstFile) {
          box.querySelector("strong").textContent = firstFile.name;
          box.querySelector("p").textContent = `已选择 ${files.length} 个文件，提交后开始上传。`;
        }
      });
      input.addEventListener("change", () => {
        if (!input.files || input.files.length === 0) return;
        const firstFile = input.files[0];
        box.querySelector("strong").textContent = firstFile.name;
        box.querySelector("p").textContent = `已选择 ${input.files.length} 个文件，提交后开始上传。`;
      });
      input.addEventListener("click", (event) => {
        event.stopPropagation();
      });
    });
  };

  const initializeShareMode = () => {
    const root = document.querySelector("[data-share-mode]");
    if (!root) return;

    const triggers = Array.from(root.querySelectorAll("[data-mode-trigger]"));
    const panels = Array.from(document.querySelectorAll("[data-mode-panel]"));
    const updateTargets = (mode) => {
      const url = new URL(window.location.href);
      url.searchParams.set("mode", mode);
      window.history.replaceState({}, "", url.toString());

      document.querySelectorAll('input[name="mode"]').forEach((input) => {
        input.value = mode;
      });
      document.querySelectorAll('input[name="redirect_to"]').forEach((input) => {
        if (input.form?.action?.includes("/admin/upload") || input.form?.action?.includes("/admin/text") || input.form?.action?.includes("/admin/items/")) {
          input.value = `${url.pathname}${url.search}`;
        }
      });
    };

    const activate = (mode) => {
      triggers.forEach((trigger) => {
        trigger.classList.toggle("is-active", trigger.dataset.modeTrigger === mode);
      });
      panels.forEach((panel) => {
        panel.classList.toggle("subpanel-hidden", panel.dataset.modePanel !== mode);
      });
      updateTargets(mode);
      window.localStorage.setItem("share_nest_mode", mode);
    };

    triggers.forEach((trigger) => {
      trigger.addEventListener("click", () => activate(trigger.dataset.modeTrigger || "file"));
    });

    const remembered = window.localStorage.getItem("share_nest_mode");
    const initialMode = new URL(window.location.href).searchParams.get("mode") || remembered || "file";
    activate(initialMode);
  };

  const initializeShareEditor = () => {
    document.addEventListener("click", (event) => {
      const trigger = event.target.closest("[data-open-share-editor]");
      if (!trigger) return;
      event.preventDefault();
      openShareEditDialog(trigger);
    });
  };

  const initializeShareDetail = () => {
    document.addEventListener("click", (event) => {
      const trigger = event.target.closest("[data-open-share-detail]");
      if (!trigger) return;
      event.preventDefault();
      openShareDetailDialog(trigger);
    });
  };

  window.AppUI = {
    notify,
    copyText,
    inferType,
    notifyFromDataset,
    buildQRCodeURL,
    hydrateQRCodeImages,
    openQRCodeDialog,
  };

  document.addEventListener("DOMContentLoaded", () => {
    notifyFromDataset(document.body);
    hydrateQRCodeImages(document);
    bindCopyButtons();
    initializePasswordButtons();
    initializeBatchSelection();
    initializeUploadForms();
    initializeDropzones();
    initializeShareMode();
    initializeShareEditor();
    initializeShareDetail();

    const modal = document.getElementById("shareModal");
    if (modal) {
      bindShareModal(modal);
    }

    document.addEventListener("click", (event) => {
      const trigger = event.target.closest("[data-open-qr]");
      if (!trigger) return;
      event.preventDefault();
      openQRCodeDialog({
        title: trigger.dataset.qrTitle || "扫码分享",
        description: trigger.dataset.qrDescription || "使用手机扫码打开这个分享链接。",
        label: trigger.dataset.qrLabel || "分享链接",
        value: trigger.dataset.qrValue || "",
        password: trigger.dataset.qrPassword || "",
      });
    });
  });
})();
