// samime_tsf.h - GoIME TSF Text Service 完整实现
//
// 实现的接口:
//   - ITfTextInputProcessorEx  (TSF 主接口)
//   - ITfKeyEventSink          (按键事件捕获)
//   - ITfCompositionSink       (组合状态管理)
//   - ITfCandidateListUIElement (候选词列表)
//   - ITfDisplayAttributeProvider (显示属性)
//
// 用法:
//   编译为 DLL: cl /EHsc /LD samime_tsf.cpp
//   注册: regsvr32 samime_tsf.dll
//   然后在系统设置中启用 "Samime" 输入法

#pragma once

#define WIN32_LEAN_AND_MEAN
#include <windows.h>
#include <ole2.h>
#include <olectl.h>
#include <msctf.h>
#include <string>
#include <vector>
#include <memory>
#include <mutex>
#include <atomic>

#pragma comment(lib, "ole32.lib")
#pragma comment(lib, "oleaut32.lib")
#pragma comment(lib, "msctf.lib")
#pragma comment(lib, "user32.lib")
#pragma comment(lib, "gdi32.lib")
#pragma comment(lib, "ws2_32.lib")

// {A1B2C3D4-E5F6-7890-ABCD-EF1234567890} - 必须唯一
// 实际部署时用 UUIDGEN 重新生成
extern const CLSID CLSID_SamimeTextService;
extern const TCHAR* const SAMIME_SERVICE_DESC;
extern const TCHAR* const SAMIME_SERVICE_NAME;

namespace samime {

// === Go 引擎通信客户端 ===

struct Candidate {
    std::wstring word;
    std::wstring pinyin;
    double score;
    std::string source;
};

class GoEngineClient {
public:
    GoEngineClient();
    ~GoEngineClient();

    bool connect();
    void disconnect();
    bool isConnected() const { return sock_ != INVALID_SOCKET; }

    // 搜索候选词
    std::vector<Candidate> search(const std::wstring& preedit);

    // 提交选定的词
    bool commit(const std::wstring& word, const std::wstring& pinyin);

    // 重置上下文
    void reset();

    // 状态查询
    std::wstring status();

private:
    bool ensureConnected();
    std::string sendRequest(const std::string& json);
    std::string readResponse();

    static std::string wideToUtf8(const std::wstring& w);
    static std::wstring utf8ToWide(const std::string& s);
    static std::string escapeJson(const std::string& s);

    SOCKET sock_;
    std::mutex mu_;
};

// === 候选词窗口 ===

class CandidateWindow {
public:
    CandidateWindow();
    ~CandidateWindow();

    bool create(HWND parentHwnd);
    void destroy();
    void show();
    void hide();
    bool isVisible() const { return hwnd_ != nullptr && ::IsWindowVisible(hwnd_); }

    void setCandidates(const std::vector<Candidate>& cands, int selectedIndex);
    void setSelectedIndex(int idx);
    int getSelectedIndex() const { return selectedIndex_; }

    // 窗口位置（通常在光标附近）
    void setPosition(int x, int y);
    void getPosition(int* x, int* y) const;

    HWND hwnd() const { return hwnd_; }

private:
    static LRESULT CALLBACK wndProc(HWND, UINT, WPARAM, LPARAM);
    LRESULT onPaint(HWND hwnd);
    void onLButtonDown(int x, int y);

    HWND hwnd_ = nullptr;
    int x_ = 0, y_ = 0;
    int selectedIndex_ = 0;
    std::vector<Candidate> candidates_;
    HFONT font_ = nullptr;

    static const TCHAR* const CLASS_NAME;
    static ATOM classAtom_;
    static ATOM registerClass();
};

// === 主 TextService 实现 ===

class SamimeTextService :
    public ITfTextInputProcessorEx,
    public ITfKeyEventSink,
    public ITfCompositionSink,
    public ITfCandidateListUIElement
{
public:
    SamimeTextService();
    ~SamimeTextService();

    // === IUnknown ===
    STDMETHODIMP QueryInterface(REFIID riid, void** ppv) override;
    STDMETHODIMP_(ULONG) AddRef() override;
    STDMETHODIMP_(ULONG) Release() override;

    // === ITfTextInputProcessor ===
    STDMETHODIMP Activate(ITfThreadMgr* ptm, TfClientId tid) override;
    STDMETHODIMP Deactivate() override;

    // === ITfTextInputProcessorEx ===
    STDMETHODIMP ActivateEx(ITfThreadMgr* ptm, TfClientId tid, DWORD dwFlags) override;

    // === ITfKeyEventSink ===
    STDMETHODIMP OnSetFocus(BOOL fForeground) override;
    STDMETHODIMP OnTestKeyDown(ITfContext* pic, WPARAM wParam, LPARAM lParam, BOOL* pfEaten) override;
    STDMETHODIMP OnKeyDown(ITfContext* pic, WPARAM wParam, LPARAM lParam, BOOL* pfEaten) override;
    STDMETHODIMP OnTestKeyUp(ITfContext* pic, WPARAM wParam, LPARAM lParam, BOOL* pfEaten) override;
    STDMETHODIMP OnKeyUp(ITfContext* pic, WPARAM wParam, LPARAM lParam, BOOL* pfEaten) override;
    STDMETHODIMP OnPreservedKey(ITfContext* pic, REFGUID rguid, BOOL* pfEaten) override;

    // === ITfCompositionSink ===
    STDMETHODIMP OnCompositionTerminated(TfEditCookie ecWrite, ITfComposition* pComposition) override;

    // === ITfUIElement / ITfUIElementMgr interfaces (候选词 UI) ===
    // ITfUIElement
    STDMETHODIMP GetDescription(BSTR* pbstr) override;
    STDMETHODIMP GetGUID(GUID* pguid) override;
    STDMETHODIMP Show(BOOL bShow) override;
    STDMETHODIMP IsShown(BOOL* pbShow) override;
    // ITfCandidateListUIElement
    STDMETHODIMP GetUpdatedFlags(DWORD* pdwFlags) override;
    STDMETHODIMP GetDocumentMgr(ITfDocumentMgr** ppdim) override;
    STDMETHODIMP GetCount(UINT* pcCandidateList) override;
    STDMETHODIMP GetSelection(UINT* puIndex) override;
    STDMETHODIMP GetString(UINT uIndex, BSTR* pbstr) override;
    STDMETHODIMP GetPageIndex(UINT* pIndex, UINT uSize, UINT* puPageCnt) override;
    STDMETHODIMP SetPageIndex(UINT* pIndex, UINT uPageCnt) override;
    STDMETHODIMP GetCurrentPage(UINT* puPage) override;

private:
    // 内部辅助
    bool handleCharacter(ITfContext* ctx, wchar_t ch);
    bool handleBackspace(ITfContext* ctx);
    bool handleReturn(ITfContext* ctx);
    bool handleEscape(ITfContext* ctx);
    bool handleSpace(ITfContext* ctx);
    bool handleDigit(ITfContext* ctx, int digit);  // 1-9 选词

    void updatePreedit(ITfContext* ctx);
    void showCandidates(ITfContext* ctx);
    bool commitCandidate(ITfContext* ctx, int idx);
    void reset();

    HRESULT insertText(ITfContext* ctx, const std::wstring& text);
    HRESULT setPreeditText(ITfContext* ctx, const std::wstring& text);

    std::atomic<ULONG> refCount_;
    ITfThreadMgr* threadMgr_ = nullptr;
    TfClientId clientId_ = TF_CLIENTID_NULL;
    ITfComposition* composition_ = nullptr;
    DWORD keyEventSinkCookie_ = TF_INVALID_COOKIE;

    GoEngineClient engine_;
    std::wstring preedit_;
    std::vector<Candidate> candidates_;
    int selectedIdx_ = 0;

    CandidateWindow candWindow_;
    DWORD threadMgrSourceCookie_ = TF_INVALID_COOKIE;
};

// === Class Factory ===
class SamimeClassFactory : public IClassFactory {
public:
    STDMETHODIMP QueryInterface(REFIID riid, void** ppv) override;
    STDMETHODIMP_(ULONG) AddRef() override { return 1; }
    STDMETHODIMP_(ULONG) Release() override { return 1; }
    STDMETHODIMP CreateInstance(IUnknown* pUnkOuter, REFIID riid, void** ppv) override;
    STDMETHODIMP LockServer(BOOL fLock) override { return S_OK; }
};

} // namespace samime
