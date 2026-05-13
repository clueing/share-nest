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

  const escapeHTML = (value) =>
    value
      .replaceAll("&", "&amp;")
      .replaceAll("<", "&lt;")
      .replaceAll(">", "&gt;")
      .replaceAll('"', "&quot;")
      .replaceAll("'", "&#39;");

  const highlightHTMLLike = (source) => {
    const comments = [];
    let text = source.replace(/<!--[\s\S]*?-->/g, (match) => {
      const token = `@@HTMLCOMMENT${comments.length}@@`;
      comments.push(`<span class="syntax-comment">${escapeHTML(match)}</span>`);
      return token;
    });

    text = escapeHTML(text).replace(
      /(&lt;\/?)([A-Za-z][\w:-]*)([\s\S]*?)(&gt;)/g,
      (_, open, tag, attrs, close) => {
        const renderedAttrs = attrs.replace(
          /([A-Za-z_:][\w:.-]*)(\s*=\s*)(&quot;.*?&quot;|&#39;.*?&#39;|[^\s"'=<>`]+)/g,
          (_, name, eq, value) =>
            `<span class="syntax-attr">${name}</span>${eq}<span class="syntax-value">${value}</span>`
        );
        return `${open}<span class="syntax-tag">${tag}</span>${renderedAttrs}${close}`;
      }
    );

    comments.forEach((html, index) => {
      text = text.replace(`@@HTMLCOMMENT${index}@@`, html);
    });
    return text;
  };

  const highlightGenericCode = (source, language) => {
    const tokens = [];
    const stash = (pattern, className) => {
      source = source.replace(pattern, (match) => {
        const token = `@@TOKEN${tokens.length}@@`;
        tokens.push(`<span class="syntax-${className}">${escapeHTML(match)}</span>`);
        return token;
      });
    };

    const commentPatterns = [
      /\/\*[\s\S]*?\*\//g,
      /(^|[^:])\/\/.*$/gm,
      /(^|\s)#.*$/gm,
      /--.*$/gm,
    ];
    commentPatterns.forEach((pattern) => stash(pattern, "comment"));
    stash(/"(?:\\.|[^"\\])*"|'(?:\\.|[^'\\])*'|`(?:\\.|[^`\\])*`/g, "string");

    let highlighted = escapeHTML(source);
    const keywordGroups = {
      javascript: /\b(await|break|case|catch|class|const|continue|debugger|default|delete|else|export|extends|finally|for|from|function|if|import|in|instanceof|let|new|of|return|super|switch|throw|try|typeof|var|void|while|yield)\b/g,
      typescript: /\b(abstract|any|as|asserts|async|await|boolean|break|case|catch|class|const|continue|debugger|declare|default|delete|else|enum|export|extends|finally|for|from|function|if|implements|import|in|infer|interface|is|keyof|let|module|namespace|new|never|of|private|protected|public|readonly|return|satisfies|static|string|super|switch|throw|try|type|typeof|unknown|var|void|while)\b/g,
      go: /\b(break|case|chan|const|continue|default|defer|else|fallthrough|for|func|go|goto|if|import|interface|map|package|range|return|select|struct|switch|type|var)\b/g,
      python: /\b(and|as|assert|async|await|break|class|continue|def|del|elif|else|except|finally|for|from|global|if|import|in|is|lambda|nonlocal|not|or|pass|raise|return|try|while|with|yield)\b/g,
      java: /\b(abstract|assert|boolean|break|byte|case|catch|class|const|continue|default|do|double|else|enum|extends|final|finally|float|for|if|implements|import|instanceof|int|interface|long|native|new|package|private|protected|public|return|short|static|strictfp|super|switch|synchronized|this|throw|throws|try|void|volatile|while)\b/g,
      c: /\b(auto|break|case|char|const|continue|default|do|double|else|enum|extern|float|for|goto|if|inline|int|long|register|restrict|return|short|signed|sizeof|static|struct|switch|typedef|union|unsigned|void|volatile|while)\b/g,
      cpp: /\b(alignas|alignof|auto|bool|break|case|catch|char|class|const|constexpr|continue|default|delete|do|double|else|enum|explicit|export|extern|false|float|for|friend|goto|if|inline|int|long|mutable|namespace|new|nullptr|operator|private|protected|public|register|return|short|signed|sizeof|static|struct|switch|template|this|throw|true|try|typedef|typename|union|unsigned|using|virtual|void|volatile|while)\b/g,
      rust: /\b(as|async|await|break|const|continue|crate|dyn|else|enum|extern|false|fn|for|if|impl|in|let|loop|match|mod|move|mut|pub|ref|return|Self|self|static|struct|super|trait|true|type|unsafe|use|where|while)\b/g,
      bash: /\b(case|do|done|elif|else|esac|fi|for|function|if|in|local|return|select|then|until|while)\b/g,
      sql: /\b(ADD|ALL|ALTER|AND|AS|ASC|BETWEEN|BY|CASE|CREATE|DELETE|DESC|DISTINCT|DROP|ELSE|END|EXISTS|FROM|GROUP|HAVING|IN|INNER|INSERT|INTO|IS|JOIN|LEFT|LIKE|LIMIT|NOT|NULL|ON|OR|ORDER|OUTER|PRIMARY|RIGHT|SELECT|SET|TABLE|THEN|UNION|UPDATE|VALUES|WHEN|WHERE)\b/gi,
      css: /\b(@media|@supports|@keyframes|from|to|important)\b/g,
      json: /\b(true|false|null)\b/g,
      yaml: /\b(true|false|null|yes|no|on|off)\b/gi,
      xml: /\b(version|encoding)\b/g,
      ini: /\b(true|false|on|off|yes|no)\b/gi,
      toml: /\b(true|false)\b/g,
    };

    const keywordPattern = keywordGroups[language];
    if (keywordPattern) {
      highlighted = highlighted.replace(keywordPattern, `<span class="syntax-keyword">$&</span>`);
    }
    highlighted = highlighted
      .replace(/\b\d+(?:\.\d+)?\b/g, `<span class="syntax-number">$&</span>`)
      .replace(/\b(true|false|null)\b/gi, `<span class="syntax-boolean">$&</span>`)
      .replace(/([{}()[\]=<>!:+\-/*|&.,;]+)/g, `<span class="syntax-operator">$1</span>`);

    tokens.forEach((token, index) => {
      highlighted = highlighted.replaceAll(`@@TOKEN${index}@@`, token);
    });
    return highlighted;
  };

  const highlightElement = (element) => {
    if (!element || element.dataset.highlighted === "true") return;
    const language = (element.dataset.codeLanguage || "text").toLowerCase();
    const target = element.querySelector("code") || element;
    const source = target.textContent || "";

    if (!source.trim()) {
      element.dataset.highlighted = "true";
      return;
    }

    if (language === "html" || language === "xml") {
      target.innerHTML = highlightHTMLLike(source);
    } else if (language !== "text" && language !== "markdown") {
      target.innerHTML = highlightGenericCode(source, language);
    } else {
      target.textContent = source;
    }
    element.dataset.highlighted = "true";
  };

  const highlightCodeBlocks = (root = document) => {
    if (window.Prism?.highlightAllUnder) {
      window.Prism.highlightAllUnder(root);
      return;
    }
    root.querySelectorAll("[data-code-language]").forEach((element) => {
      highlightElement(element);
    });
  };

  window.AppUI = {
    notify,
    copyText,
    inferType,
    notifyFromDataset,
    highlightCodeBlocks,
  };

  document.addEventListener("DOMContentLoaded", () => {
    notifyFromDataset(document.body);
    highlightCodeBlocks(document);
  });
})();
