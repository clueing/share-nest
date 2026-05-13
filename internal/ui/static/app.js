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

    const passwordContent = document.createElement("div");
    passwordContent.className = "qr-meta-group";
    if (password) {
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
    meta.append(metaContent);
    if (password) {
      meta.append(passwordContent);
    }
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
    if (/(成功|完成|已|复制)/.test(message)) return "success";
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
