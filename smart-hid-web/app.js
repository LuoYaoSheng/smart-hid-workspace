/* Smart HID — Landing interactions
   零依赖：移动端菜单、滚动渐现、锚点关闭菜单。 */
(function () {
  "use strict";

  // ---- 移动端菜单开关 ----
  var toggle = document.getElementById("nav-toggle");
  var links = document.getElementById("nav-links");
  if (toggle && links) {
    toggle.addEventListener("click", function () {
      links.classList.toggle("open");
    });
    // 点击任一链接后收起
    links.querySelectorAll("a").forEach(function (a) {
      a.addEventListener("click", function () { links.classList.remove("open"); });
    });
  }

  // ---- 滚动渐现（IntersectionObserver，降级为直接显示）----
  var revealEls = document.querySelectorAll(
    ".section-head, .flow-node, .feature-card, .comp-card, .why-item, .how-note img, .note-list li, .code-demo, .fb-card"
  );
  revealEls.forEach(function (el) { el.classList.add("reveal"); });

  if ("IntersectionObserver" in window) {
    var io = new IntersectionObserver(function (entries) {
      entries.forEach(function (e) {
        if (e.isIntersecting) {
          e.target.classList.add("in");
          io.unobserve(e.target);
        }
      });
    }, { threshold: 0.12, rootMargin: "0px 0px -40px 0px" });
    revealEls.forEach(function (el) { io.observe(el); });
  } else {
    revealEls.forEach(function (el) { el.classList.add("in"); });
  }

  // ---- 需求与反馈表单（FB-1，匿名提交到 Cloud 公开端点）----
  var fbForm = document.getElementById("fb-form");
  if (fbForm) {
    var fbOk = document.getElementById("fb-ok");
    var fbMsg = document.getElementById("fb-msg");
    var fbSubmit = document.getElementById("fb-submit");

    // API 地址：同源部署用默认相对路径；独立托管时可用 localStorage 覆盖（与 portal/admin 同键）
    var fbBase = "/api/v1";
    try {
      var fbOverride = localStorage.getItem("smarthid_cloud_base");
      if (fbOverride) fbBase = fbOverride;
    } catch (e) { /* file:// 或隐私模式下降级为同源 */ }

    function fbErr(m) {
      fbMsg.className = "fb-msg err";
      fbMsg.textContent = m;
      fbSubmit.disabled = false;
      fbSubmit.textContent = "提交反馈";
    }

    fbForm.addEventListener("submit", function (ev) {
      ev.preventDefault();
      fbMsg.className = "fb-msg";
      fbMsg.textContent = "";

      var els = fbForm.elements;
      var checked = fbForm.querySelector('input[name="category"]:checked');
      var payload = {
        category: checked ? checked.value : "feature",
        title: els["title"].value.trim(),
        body: els["body"].value.trim(),
        contact: els["contact"].value.trim(),
        website: els["website"].value // honeypot：正常恒空
      };
      if (payload.title.length < 3) { fbErr("标题至少 3 个字"); return; }
      if (payload.body.length < 5) { fbErr("详细说明至少 5 个字"); return; }

      fbSubmit.disabled = true;
      fbSubmit.textContent = "提交中…";
      fetch(fbBase + "/feedback", {
        method: "POST",
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify(payload)
      }).then(function (res) {
        return res.json().then(function (j) { return { status: res.status, json: j }; });
      }).then(function (r) {
        if (r.status === 201) {
          document.getElementById("fb-ok-id").textContent = r.json.feedback_id || "";
          fbForm.hidden = true;
          fbOk.hidden = false;
        } else if (r.status === 429) {
          fbErr("提交太频繁，请 1 小时后再试");
        } else {
          fbErr((r.json && r.json.message) ? r.json.message : "提交失败（" + r.status + "）");
        }
      }).catch(function () {
        fbErr("无法连接服务器：请确认 Cloud API 在线（同源部署，或设置 smarthid_cloud_base）");
      });
    });

    var fbAgain = document.getElementById("fb-again");
    if (fbAgain) {
      fbAgain.addEventListener("click", function () {
        fbForm.reset();
        fbForm.hidden = false;
        fbOk.hidden = true;
        fbSubmit.disabled = false;
        fbSubmit.textContent = "提交反馈";
        fbMsg.textContent = "";
      });
    }
  }
})();
