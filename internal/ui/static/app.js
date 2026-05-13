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
    meta.append(metaLabel, code, note, actions);
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

  const copyText = async (value, successMessage = "复制成功") => {
    if (!value) return false;

    try {
      await navigator.clipboard.writeText(value);
      notify({ message: successMessage, type: "success" });
      return true;
    } catch (_) {
      try {
        const area = document.createElement("textarea");
        area.value = value;
        area.setAttribute("readonly", "");
        area.style.position = "absolute";
        area.style.left = "-9999px";
        document.body.appendChild(area);
        area.select();
        const copied = document.execCommand("copy");
        document.body.removeChild(area);
        if (!copied) {
          throw new Error("copy failed");
        }
        notify({ message: successMessage, type: "success" });
        return true;
      } catch (_) {
        notify({ message: "复制失败，请手动复制。", type: "error" });
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
      });
    });
  });
})();
