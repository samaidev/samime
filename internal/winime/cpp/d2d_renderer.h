// d2d_renderer.h - Direct2D 渲染器（硬件加速）
//
// 用 Direct2D + DirectWrite 替代 GDI 渲染候选词窗口
// 优势：
//   - 硬件加速（GPU 渲染）
//   - 抗锯齿文字（ClearType + DirectWrite）
//   - 圆角矩形原生支持
//   - 渐变画刷原生支持
//   - 性能更好（避免 GDI 双缓冲）
//
// 编译需要链接: d2d1.lib dwrite.lib

#pragma once

#include <windows.h>
#include <d2d1.h>
#include <dwrite.h>
#include <string>
#include <vector>
#include <memory>

#pragma comment(lib, "d2d1.lib")
#pragma comment(lib, "dwrite.lib")

namespace samime {

struct D2DCandidate {
    std::wstring word;
    std::wstring pinyin;
    double score;
    std::string source;
};

// Direct2D 渲染器
class D2DRenderer {
public:
    D2DRenderer();
    ~D2DRenderer();

    // 初始化（绑定到 HWND）
    bool initialize(HWND hwnd);

    // 释放资源
    void release();

    // 渲染候选词列表
    // width/height: 渲染区域大小
    // candidates: 候选词列表
    // selectedIndex: 当前选中项
    // animProgress: 动画进度 0.0~1.0
    // prevSelectedIndex: 上一次选中项（用于动画）
    void render(int width, int height,
                const std::vector<D2DCandidate>& candidates,
                int selectedIndex,
                float animProgress,
                int prevSelectedIndex);

    // 窗口大小变化时调用
    void onResize(int width, int height);

private:
    // 创建设备无关资源（D2D Factory, DWrite Factory, TextFormat）
    bool createDeviceIndependentResources();
    // 创建设备相关资源（HwndRenderTarget, Brushes）
    bool createDeviceResources(HWND hwnd);
    // 释放设备相关资源
    void discardDeviceResources();

    // 颜色辅助
    D2D1_COLOR_F colorInterpolate(const D2D1_COLOR_F& from, const D2D1_COLOR_F& to, float t);

    HWND hwnd_ = nullptr;
    bool initialized_ = false;

    // 设备无关资源
    ID2D1Factory* d2dFactory_ = nullptr;
    IDWriteFactory* dwriteFactory_ = nullptr;
    IDWriteTextFormat* textFormat_ = nullptr;       // 候选词字体
    IDWriteTextFormat* smallTextFormat_ = nullptr;  // 拼音提示字体

    // 设备相关资源
    ID2D1HwndRenderTarget* renderTarget_ = nullptr;

    // 画刷（重复使用，避免每帧创建）
    ID2D1SolidColorBrush* selectedBgBrush_ = nullptr;     // 选中背景
    ID2D1SolidColorBrush* normalBgBrush_ = nullptr;       // 普通背景
    ID2D1SolidColorBrush* gradientTopBrush_ = nullptr;    // 渐变顶部
    ID2D1SolidColorBrush* gradientBottomBrush_ = nullptr; // 渐变底部
    ID2D1SolidColorBrush* selectedTextBrush_ = nullptr;   // 选中文字
    ID2D1SolidColorBrush* normalTextBrush_ = nullptr;     // 普通文字
    ID2D1SolidColorBrush* indexBgBrush_ = nullptr;        // 序号背景
    ID2D1SolidColorBrush* indexTextBrush_ = nullptr;      // 序号文字
    ID2D1SolidColorBrush* pinyinBrush_ = nullptr;         // 拼音提示
    ID2D1SolidColorBrush* animBrush_ = nullptr;           // 动画用（动态颜色）

    // 配色（Windows 11 风格）
    static constexpr D2D1_COLOR_F COLOR_SELECTED_BG = { 0.0f, 0.47f, 0.84f, 1.0f };  // #0078D4
    static constexpr D2D1_COLOR_F COLOR_NORMAL_BG = { 0.98f, 0.98f, 0.99f, 1.0f };
    static constexpr D2D1_COLOR_F COLOR_SELECTED_TEXT = { 1.0f, 1.0f, 1.0f, 1.0f };
    static constexpr D2D1_COLOR_F COLOR_NORMAL_TEXT = { 0.2f, 0.2f, 0.2f, 1.0f };
    static constexpr D2D1_COLOR_F COLOR_PINYIN = { 0.55f, 0.55f, 0.55f, 1.0f };
    static constexpr D2D1_COLOR_F COLOR_INDEX_BG = { 0.94f, 0.94f, 0.94f, 1.0f };
    static constexpr D2D1_COLOR_F COLOR_INDEX_TEXT = { 0.47f, 0.47f, 0.47f, 1.0f };
};

} // namespace samime
