<!-- BEGIN:nextjs-agent-rules -->
# This is NOT the Next.js you know

This version has breaking changes — APIs, conventions, and file structure may all differ from your training data. Read the relevant guide in `node_modules/next/dist/docs/` before writing any code. Heed deprecation notices.
<!-- END:nextjs-agent-rules -->

## 界面与交互约定（用户要求）

- 使用清爽的中性底色、明确的品牌强调色与统一状态色，兼顾浅色与深色。精修字体层级、间距、图标，信息紧凑但不拥挤。
- 优先连续分区、对象列表、主从详情与就地操作，避免卡片套卡片、KPI 卡墙和传统后台模板感。功能分组使用标题、分隔线与留白表达层次。
- 主动选用并适配成熟组件，优先复用项目已安装的 shadcn/ui / Base UI；根据实际需求评估 Aceternity UI、Magic UI、Origin UI 等，不为装饰重复引入组件库。
- 使用贴合业务的视觉资产与轻量动效；动效服务反馈和空间连续性，不遮挡内容、不挤动布局、不拖慢操作。支持减少动态效果。
- 按正式产品完成相关操作闭环，包括搜索定位、详情切换、返回，以及加载、空、错误和离线状态。保持真实数据和业务边界。
- 文案自然准确，不堆术语，不在产品界面解释设计理念。
- 兼顾响应式与键盘操作；通过实际页面和浏览器交互验证，不能只交付截图。
