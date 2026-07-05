// d2d_renderer.cpp - Direct2D 渲染器实现
//
// 用 Direct2D + DirectWrite 渲染候选词窗口
// 硬件加速，抗锯齿，原生圆角矩形和渐变

#include "d2d_renderer.h"
#include <cmath>
#include <algorithm>

namespace samime {

// 辅助宏：SafeRelease
template <class T>
inline void SafeRelease(T** ppT) {
    if (*ppT) {
        (*ppT)->Release();
        *ppT = nullptr;
    }
}

D2DRenderer::D2DRenderer() {}

D2DRenderer::~D2DRenderer() {
    release();
}

bool D2DRenderer::initialize(HWND hwnd) {
    hwnd_ = hwnd;
    if (!createDeviceIndependentResources()) {
        return false;
    }
    if (!createDeviceResources(hwnd)) {
        return false;
    }
    initialized_ = true;
    return true;
}

void D2DRenderer::release() {
    discardDeviceResources();
    SafeRelease(&d2dFactory_);
    SafeRelease(&dwriteFactory_);
    SafeRelease(&textFormat_);
    SafeRelease(&smallTextFormat_);
    initialized_ = false;
}

bool D2DRenderer::createDeviceIndependentResources() {
    // D2D Factory
    HRESULT hr = D2D1CreateFactory(D2D1_FACTORY_TYPE_SINGLE_THREADED, &d2dFactory_);
    if (FAILED(hr)) return false;

    // DWrite Factory
    hr = DWriteCreateFactory(DWRITE_FACTORY_TYPE_SHARED, __uuidof(IDWriteFactory),
                              reinterpret_cast<IUnknown**>(&dwriteFactory_));
    if (FAILED(hr)) return false;

    // 候选词字体（Microsoft YaHei UI, 16px）
    hr = dwriteFactory_->CreateTextFormat(
        L"Microsoft YaHei UI",
        nullptr,
        DWRITE_FONT_WEIGHT_REGULAR,
        DWRITE_FONT_STYLE_NORMAL,
        DWRITE_FONT_STRETCH_NORMAL,
        16.0f,
        L"zh-CN",
        &textFormat_);
    if (FAILED(hr)) return false;
    textFormat_->SetTextAlignment(DWRITE_TEXT_ALIGNMENT_LEADING);
    textFormat_->SetParagraphAlignment(DWRITE_PARAGRAPH_ALIGNMENT_CENTER);

    // 拼音提示字体（12px）
    hr = dwriteFactory_->CreateTextFormat(
        L"Microsoft YaHei UI",
        nullptr,
        DWRITE_FONT_WEIGHT_REGULAR,
        DWRITE_FONT_STYLE_NORMAL,
        DWRITE_FONT_STRETCH_NORMAL,
        12.0f,
        L"zh-CN",
        &smallTextFormat_);
    if (FAILED(hr)) return false;
    smallTextFormat_->SetTextAlignment(DWRITE_TEXT_ALIGNMENT_TRAILING);
    smallTextFormat_->SetParagraphAlignment(DWRITE_PARAGRAPH_ALIGNMENT_CENTER);

    return true;
}

bool D2DRenderer::createDeviceResources(HWND hwnd) {
    if (!d2dFactory_) return false;

    RECT rc;
    GetClientRect(hwnd, &rc);
    D2D1_SIZE_U size = D2D1::SizeU(rc.right - rc.left, rc.bottom - rc.top);

    HRESULT hr = d2dFactory_->CreateHwndRenderTarget(
        D2D1::RenderTargetProperties(),
        D2D1::HwndRenderTargetProperties(hwnd, size),
        &renderTarget_);
    if (FAILED(hr)) return false;

    // 创建画刷
    renderTarget_->CreateSolidColorBrush(COLOR_SELECTED_BG, &selectedBgBrush_);
    renderTarget_->CreateSolidColorBrush(COLOR_NORMAL_BG, &normalBgBrush_);
    renderTarget_->CreateSolidColorBrush(COLOR_SELECTED_TEXT, &selectedTextBrush_);
    renderTarget_->CreateSolidColorBrush(COLOR_NORMAL_TEXT, &normalTextBrush_);
    renderTarget_->CreateSolidColorBrush(COLOR_INDEX_BG, &indexBgBrush_);
    renderTarget_->CreateSolidColorBrush(COLOR_INDEX_TEXT, &indexTextBrush_);
    renderTarget_->CreateSolidColorBrush(COLOR_PINYIN, &pinyinBrush_);
    renderTarget_->CreateSolidColorBrush(D2D1::ColorF(D2D1::ColorF::White), &animBrush_);

    return true;
}

void D2DRenderer::discardDeviceResources() {
    SafeRelease(&renderTarget_);
    SafeRelease(&selectedBgBrush_);
    SafeRelease(&normalBgBrush_);
    SafeRelease(&gradientTopBrush_);
    SafeRelease(&gradientBottomBrush_);
    SafeRelease(&selectedTextBrush_);
    SafeRelease(&normalTextBrush_);
    SafeRelease(&indexBgBrush_);
    SafeRelease(&indexTextBrush_);
    SafeRelease(&pinyinBrush_);
    SafeRelease(&animBrush_);
}

void D2DRenderer::onResize(int width, int height) {
    if (renderTarget_) {
        renderTarget_->Resize(D2D1::SizeU(width, height));
    }
}

D2D1_COLOR_F D2DRenderer::colorInterpolate(const D2D1_COLOR_F& from, const D2D1_COLOR_F& to, float t) {
    return D2D1::ColorF(
        from.r + (to.r - from.r) * t,
        from.g + (to.g - from.g) * t,
        from.b + (to.b - from.b) * t,
        from.a + (to.a - from.a) * t
    );
}

void D2DRenderer::render(int width, int height,
                          const std::vector<D2DCandidate>& candidates,
                          int selectedIndex,
                          float animProgress,
                          int prevSelectedIndex) {
    if (!renderTarget_ || !initialized_) return;

    // 检查设备是否丢失（D2D 在显示模式变化等情况下需要重建）
    HRESULT hr = renderTarget_->EndDraw();
    if (hr == D2DERR_RECREATE_TARGET) {
        discardDeviceResources();
        createDeviceResources(hwnd_);
    }

    renderTarget_->BeginDraw();

    // 清除背景（渐变效果用多层矩形模拟）
    renderTarget_->Clear(COLOR_NORMAL_BG);

    // 顶部高光条
    D2D1_RECT_F topRect = D2D1::RectF(0, 0, width, 2);
    ID2D1SolidColorBrush* topBrush = nullptr;
    renderTarget_->CreateSolidColorBrush(D2D1::ColorF(0.86f, 0.90f, 0.94f, 1.0f), &topBrush);
    renderTarget_->FillRectangle(topRect, topBrush);
    SafeRelease(&topBrush);

    int lineHeight = 28;
    int padding = 8;
    int indexWidth = 24;

    int y = padding;
    int count = (int)(std::min)(candidates.size(), (size_t)9);

    for (int i = 0; i < count; i++) {
        D2D1_RECT_F itemRect = D2D1::RectF(padding, y, width - padding, y + lineHeight);

        bool isCurrentSelected = (i == selectedIndex);
        bool isPrevSelected = (i == prevSelectedIndex && prevSelectedIndex != selectedIndex);

        // === 选中背景（带动画淡入淡出）===
        if (isCurrentSelected || isPrevSelected) {
            float selAmount = 0.0f;
            if (isCurrentSelected) {
                selAmount = animProgress;
            } else if (isPrevSelected) {
                selAmount = 1.0f - animProgress;
            }

            // 颜色插值
            D2D1_COLOR_F bgColor = colorInterpolate(COLOR_NORMAL_BG, COLOR_SELECTED_BG, selAmount);
            ID2D1SolidColorBrush* bgBrush = nullptr;
            renderTarget_->CreateSolidColorBrush(bgColor, &bgBrush);

            // 圆角矩形（D2D 原生支持）
            D2D1_ROUNDED_RECT roundedRect = D2D1::RoundedRect(itemRect, 6, 6);
            renderTarget_->FillRoundedRectangle(roundedRect, bgBrush);
            SafeRelease(&bgBrush);
        }

        // === 序号徽章（圆形）===
        D2D1_ELLIPSE indexEllipse = D2D1::Ellipse(
            D2D1::Point2F(itemRect.left + 14, itemRect.top + 14),
            10, 10
        );

        bool showSelectedStyle = (isCurrentSelected && animProgress > 0.5f);
        if (!showSelectedStyle) {
            // 普通时序号背景圆
            renderTarget_->FillEllipse(indexEllipse, indexBgBrush_);
        }

        // 序号文字
        std::wstring idxText = std::to_wstring(i + 1);
        D2D1_RECT_F idxRect = D2D1::RectF(itemRect.left + 4, itemRect.top + 4,
                                           itemRect.left + 24, itemRect.top + 24);
        renderTarget_->DrawTextW(idxText.c_str(), (UINT32)idxText.size(),
                                  smallTextFormat_, idxRect,
                                  showSelectedStyle ? selectedTextBrush_ : indexTextBrush_);

        // === 候选词文字 ===
        D2D1_RECT_F wordRect = D2D1::RectF(itemRect.left + indexWidth + 4, itemRect.top,
                                            itemRect.right - 4, itemRect.bottom);
        std::wstring word = candidates[i].word;
        renderTarget_->DrawTextW(word.c_str(), (UINT32)word.size(),
                                  textFormat_, wordRect,
                                  showSelectedStyle ? selectedTextBrush_ : normalTextBrush_);

        // === 拼音提示（右对齐，浅色）===
        if (!showSelectedStyle && !candidates[i].pinyin.empty()) {
            D2D1_RECT_F pyRect = D2D1::RectF(itemRect.left + indexWidth + 4, itemRect.top,
                                              itemRect.right - 4, itemRect.bottom);
            std::wstring py = candidates[i].pinyin;
            renderTarget_->DrawTextW(py.c_str(), (UINT32)py.size(),
                                      smallTextFormat_, pyRect, pinyinBrush_);
        }

        y += lineHeight;
    }

    hr = renderTarget_->EndDraw();
    if (hr == D2DERR_RECREATE_TARGET) {
        discardDeviceResources();
        createDeviceResources(hwnd_);
    }
}

} // namespace samime
