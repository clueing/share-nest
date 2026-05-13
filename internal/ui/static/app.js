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
  };

  document.addEventListener("DOMContentLoaded", () => {
    notifyFromDataset(document.body);
  });
})();
