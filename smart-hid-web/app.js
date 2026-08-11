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
    ".section-head, .flow-node, .feature-card, .comp-card, .why-item, .how-note img, .note-list li, .code-demo"
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
})();
