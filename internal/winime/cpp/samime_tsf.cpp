// samime_tsf.cpp - GoIME TSF Text Service 完整实现
//
// 编译 (Visual Studio Build Tools):
//   cl /EHsc /std:c++17 /LD samime_tsf.cpp samime_classfactory.cpp \
//      /link ole32.lib oleaut32.lib msctf.lib user32.lib gdi32.lib ws2_32.lib
//
// 编译 (MinGW):
//   x86_64-w64-mingw32-g++ -shared -std=c++17 -o samime_tsf.dll \
//      samime_tsf.cpp samime_classfactory.cpp \
//      -lole32 -loleaut32 -lmsctf -luser32 -lgdi32 -lws2_32

#include "samime_tsf.h"
#include <strsafe.h>
#include <algorithm>
#include <sstream>
#include <winsock2.h>
#include <ws2tcpip.h>

namespace samime {

// === CLSID 和常量 ===

// {A1B2C3D4-E5F6-7890-ABCD-EF1234567890}
const CLSID CLSID_SamimeTextService = {
    0xA1B2C3D4, 0xE5F6, 0x7890,
    {0xAB, 0xCD, 0xEF, 0x12, 0x34, 0x56, 0x78, 0x90}
};

const TCHAR* const SAMIME_SERVICE_DESC = _T("Samime Chinese Input Method");
const TCHAR* const SAMIME_SERVICE_NAME = _T("Samime");

// === GoEngineClient ===

GoEngineClient::GoEngineClient() : sock_(INVALID_SOCKET) {
    WSADATA wsaData;
    WSAStartup(MAKEWORD(2, 2), &wsaData);
}

GoEngineClient::~GoEngineClient() {
    disconnect();
    WSACleanup();
}

bool GoEngineClient::connect() {
    sock_ = socket(AF_INET, SOCK_STREAM, IPPROTO_TCP);
    if (sock_ == INVALID_SOCKET) return false;

    // 设置超时
    DWORD timeout = 2000;
    setsockopt(sock_, SOL_SOCKET, SO_RCVTIMEO, (char*)&timeout, sizeof(timeout));
    setsockopt(sock_, SOL_SOCKET, SO_SNDTIMEO, (char*)&timeout, sizeof(timeout));

    // 连接命名管道（通过 localhost TCP，开发模式）
    // 实际生产应使用 winio ListenPipe 的命名管道
    sockaddr_in addr = {};
    addr.sin_family = AF_INET;
    addr.sin_port = htons(7788);
    inet_pton(AF_INET, "127.0.0.1", &addr.sin_addr);

    if (::connect(sock_, (sockaddr*)&addr, sizeof(addr)) == SOCKET_ERROR) {
        closesocket(sock_);
        sock_ = INVALID_SOCKET;
        return false;
    }
    return true;
}

void GoEngineClient::disconnect() {
    std::lock_guard<std::mutex> lock(mu_);
    if (sock_ != INVALID_SOCKET) {
        closesocket(sock_);
        sock_ = INVALID_SOCKET;
    }
}

bool GoEngineClient::ensureConnected() {
    if (sock_ != INVALID_SOCKET) return true;
    return connect();
}

std::string GoEngineClient::wideToUtf8(const std::wstring& w) {
    if (w.empty()) return "";
    int len = WideCharToMultiByte(CP_UTF8, 0, w.c_str(), (int)w.size(),
                                   nullptr, 0, nullptr, nullptr);
    std::string s(len, '\0');
    WideCharToMultiByte(CP_UTF8, 0, w.c_str(), (int)w.size(), &s[0], len, nullptr, nullptr);
    return s;
}

std::wstring GoEngineClient::utf8ToWide(const std::string& s) {
    if (s.empty()) return L"";
    int len = MultiByteToWideChar(CP_UTF8, 0, s.c_str(), (int)s.size(), nullptr, 0);
    std::wstring w(len, L'\0');
    MultiByteToWideChar(CP_UTF8, 0, s.c_str(), (int)s.size(), &w[0], len);
    return w;
}

std::string GoEngineClient::escapeJson(const std::string& s) {
    std::string out;
    out.reserve(s.size() + 8);
    for (char c : s) {
        switch (c) {
            case '"':  out += "\\\""; break;
            case '\\': out += "\\\\"; break;
            case '\n': out += "\\n"; break;
            case '\r': out += "\\r"; break;
            case '\t': out += "\\t"; break;
            default:
                if ((unsigned char)c < 0x20) {
                    char buf[8];
                    StringCchPrintfA(buf, 8, "\\u%04x", (unsigned char)c);
                    out += buf;
                } else {
                    out += c;
                }
        }
    }
    return out;
}

std::string GoEngineClient::sendRequest(const std::string& json) {
    std::lock_guard<std::mutex> lock(mu_);
    if (!ensureConnected()) return "{}";

    std::string req = json + "\n";
    int total = 0;
    while (total < (int)req.size()) {
        int n = send(sock_, req.c_str() + total, (int)req.size() - total, 0);
        if (n <= 0) {
            disconnect();
            return "{}";
        }
        total += n;
    }
    return readResponse();
}

std::string GoEngineClient::readResponse() {
    char buf[65536];
    int total = 0;
    while (total < (int)sizeof(buf) - 1) {
        int n = recv(sock_, buf + total, (int)sizeof(buf) - 1 - total, 0);
        if n <= 0) break;
        total += n;
        buf[total] = '\0';
        if (strchr(buf, '\n')) break;
    }
    buf[total < (int)sizeof(buf) ? total : (int)sizeof(buf) - 1] = '\0';
    return std::string(buf);
}

std::vector<Candidate> GoEngineClient::search(const std::wstring& preedit) {
    std::vector<Candidate> result;
    std::string preeditUtf8 = wideToUtf8(preedit);
    std::string req = "{\"method\":\"search\",\"preedit\":\"" +
                      escapeJson(preeditUtf8) + "\"}";
    std::string resp = sendRequest(req);

    // 简单 JSON 解析（避免引入 nlohmann/json 依赖）
    // 解析 candidates 数组
    size_t candsPos = resp.find("\"candidates\"");
    if (candsPos == std::string::npos) return result;

    size_t arrStart = resp.find('[', candsPos);
    size_t arrEnd = resp.find(']', arrStart);
    if (arrStart == std::string::npos || arrEnd == std::string::npos) return result;

    std::string arr = resp.substr(arrStart + 1, arrEnd - arrStart - 1);

    // 逐个解析 {word, pinyin, score, source}
    size_t pos = 0;
    while (pos < arr.size()) {
        size_t objStart = arr.find('{', pos);
        if (objStart == std::string::npos) break;
        size_t objEnd = arr.find('}', objStart);
        if (objEnd == std::string::npos) break;

        std::string obj = arr.substr(objStart, objEnd - objStart + 1);
        Candidate c;

        // 提取 Word
        size_t wPos = obj.find("\"Word\":\"");
        if (wPos != std::string::npos) {
            size_t start = wPos + 8;
            size_t end = obj.find("\"", start);
            if (end != std::string::npos) {
                c.word = utf8ToWide(obj.substr(start, end - start));
            }
        }
        // 提取 Pinyin
        size_t pPos = obj.find("\"Pinyin\":\"");
        if (pPos != std::string::npos) {
            size_t start = pPos + 10;
            size_t end = obj.find("\"", start);
            if (end != std::string::npos) {
                c.pinyin = utf8ToWide(obj.substr(start, end - start));
            }
        }
        // 提取 Score
        size_t sPos = obj.find("\"Score\":");
        if (sPos != std::string::npos) {
            c.score = atof(obj.c_str() + sPos + 7);
        }
        // 提取 Source
        size_t srcPos = obj.find("\"Source\":\"");
        if (srcPos != std::string::npos) {
            size_t start = srcPos + 10;
            size_t end = obj.find("\"", start);
            if (end != std::string::npos) {
                c.source = obj.substr(start, end - start);
            }
        }
        if (!c.word.empty()) {
            result.push_back(c);
        }
        pos = objEnd + 1;
    }
    return result;
}

bool GoEngineClient::commit(const std::wstring& word, const std::wstring& pinyin) {
    std::string req = "{\"method\":\"commit\",\"word\":\"" +
                      escapeJson(wideToUtf8(word)) +
                      "\",\"pinyin\":\"" + escapeJson(wideToUtf8(pinyin)) + "\"}";
    std::string resp = sendRequest(req);
    return resp.find("\"ok\":true") != std::string::npos;
}

void GoEngineClient::reset() {
    sendRequest("{\"method\":\"reset\"}");
}

std::wstring GoEngineClient::status() {
    std::string resp = sendRequest("{\"method\":\"status\"}");
    return utf8ToWide(resp);
}

// === CandidateWindow ===

const TCHAR* const CandidateWindow::CLASS_NAME = _T("SamimeCandidateWindow");
ATOM CandidateWindow::classAtom_ = 0;

ATOM CandidateWindow::registerClass() {
    if (classAtom_) return classAtom_;
    WNDCLASS wc = {};
    wc.lpfnWndProc = CandidateWindow::wndProc;
    wc.hInstance = GetModuleHandle(nullptr);
    wc.lpszClassName = CLASS_NAME;
    wc.hCursor = LoadCursor(nullptr, IDC_ARROW);
    wc.hbrBackground = (HBRUSH)(COLOR_WINDOW + 1);
    classAtom_ = RegisterClass(&wc);
    return classAtom_;
}

CandidateWindow::CandidateWindow() {
    registerClass();
    font_ = CreateFont(18, 0, 0, 0, FW_NORMAL, FALSE, FALSE, FALSE,
                       DEFAULT_CHARSET, OUT_DEFAULT_PRECIS,
                       CLIP_DEFAULT_PRECIS, CLEARTYPE_QUALITY,
                       DEFAULT_PITCH | FF_DONTCARE, _T("Microsoft YaHei"));
}

CandidateWindow::~CandidateWindow() {
    destroy();
    if (font_) DeleteObject(font_);
}

bool CandidateWindow::create(HWND parentHwnd) {
    hwnd_ = CreateWindowEx(
        WS_EX_TOOLWINDOW | WS_EX_TOPMOST | WS_EX_NOACTIVATE,
        CLASS_NAME, _T(""),
        WS_POPUP | WS_BORDER,
        CW_USEDEFAULT, CW_USEDEFAULT, 300, 200,
        parentHwnd, nullptr, GetModuleHandle(nullptr), this);
    return hwnd_ != nullptr;
}

void CandidateWindow::destroy() {
    if (hwnd_) {
        DestroyWindow(hwnd_);
        hwnd_ = nullptr;
    }
}

void CandidateWindow::show() {
    if (hwnd_) {
        ShowWindow(hwnd_, SW_SHOWNOACTIVATE);
    }
}

void CandidateWindow::hide() {
    if (hwnd_) {
        ShowWindow(hwnd_, SW_HIDE);
    }
}

void CandidateWindow::setCandidates(const std::vector<Candidate>& cands, int selectedIndex) {
    candidates_ = cands;
    selectedIndex_ = selectedIndex;

    if (!hwnd_) return;

    // 计算窗口大小
    HDC hdc = GetDC(hwnd_);
    HFONT oldFont = (HFONT)SelectObject(hdc, font_);
    int maxWidth = 0;
    int lineHeight = 22;
    for (size_t i = 0; i < candidates_.size() && i < 9; i++) {
        std::wstring text = std::to_wstring(i + 1) + L". " + candidates_[i].word;
        SIZE sz;
        GetTextExtentPoint32(hdc, text.c_str(), (int)text.size(), &sz);
        if (sz.cx > maxWidth) maxWidth = sz.cx;
    }
    SelectObject(hdc, oldFont);
    ReleaseDC(hwnd_, hdc);

    int width = maxWidth + 20;
    int height = lineHeight * (std::min)(candidates_.size(), (size_t)9) + 10;

    SetWindowPos(hwnd_, nullptr, x_, y_, width, height,
                 SWP_NOACTIVATE | SWP_SHOWWINDOW);
    InvalidateRect(hwnd_, nullptr, TRUE);
}

void CandidateWindow::setSelectedIndex(int idx) {
    if (idx >= 0 && idx < (int)candidates_.size()) {
        selectedIdx_ = idx;
        InvalidateRect(hwnd_, nullptr, FALSE);
    }
}

void CandidateWindow::setPosition(int x, int y) {
    x_ = x;
    y_ = y;
    if (hwnd_) {
        SetWindowPos(hwnd_, nullptr, x, y, 0, 0,
                     SWP_NOSIZE | SWP_NOACTIVATE);
    }
}

void CandidateWindow::getPosition(int* x, int* y) const {
    if (x) *x = x_;
    if (y) *y = y_;
}

LRESULT CALLBACK CandidateWindow::wndProc(HWND hwnd, UINT msg, WPARAM wp, LPARAM lp) {
    CandidateWindow* self = nullptr;
    if (msg == WM_CREATE) {
        auto cs = (LPCREATESTRUCT)lp;
        SetWindowLongPtr(hwnd, GWLP_USERDATA, (LONG_PTR)cs->lpCreateParams);
        self = (CandidateWindow*)cs->lpCreateParams;
    } else {
        self = (CandidateWindow*)GetWindowLongPtr(hwnd, GWLP_USERDATA);
    }
    if (!self) {
        return DefWindowProc(hwnd, msg, wp, lp);
    }

    switch (msg) {
        case WM_PAINT:
            return self->onPaint(hwnd);
        case WM_LBUTTONDOWN:
            self->onLButtonDown(GET_X_LPARAM(lp), GET_Y_LPARAM(lp));
            return 0;
        case WM_ERASEBKGND:
            return 1;
    }
    return DefWindowProc(hwnd, msg, wp, lp);
}

LRESULT CandidateWindow::onPaint(HWND hwnd) {
    PAINTSTRUCT ps;
    HDC hdc = BeginPaint(hwnd, &ps);

    RECT rc;
    GetClientRect(hwnd, &rc);

    // 双缓冲
    HDC memDC = CreateCompatibleDC(hdc);
    HBITMAP memBmp = CreateCompatibleBitmap(hdc, rc.right, rc.bottom);
    HBITMAP oldBmp = (HBITMAP)SelectObject(memDC, memBmp);

    FillRect(memDC, &rc, (HBRUSH)GetStockObject(WHITE_BRUSH));

    HFONT oldFont = (HFONT)SelectObject(memDC, font_);
    SetBkMode(memDC, TRANSPARENT);

    int lineHeight = 22;
    int y = 5;
    for (size_t i = 0; i < candidates_.size() && i < 9; i++) {
        RECT itemRc = {5, y, rc.right - 5, y + lineHeight};
        if ((int)i == selectedIdx_) {
            HBRUSH br = CreateSolidBrush(RGB(51, 153, 255));
            FillRect(memDC, &itemRc, br);
            DeleteObject(br);
            SetTextColor(memDC, RGB(255, 255, 255));
        } else {
            SetTextColor(memDC, RGB(0, 0, 0));
        }

        std::wstring text = std::to_wstring(i + 1) + L". " + candidates_[i].word;
        DrawText(memDC, text.c_str(), (int)text.size(), &itemRc,
                 DT_LEFT | DT_VCENTER | DT_SINGLELINE);
        y += lineHeight;
    }

    SelectObject(memDC, oldFont);
    BitBlt(hdc, 0, 0, rc.right, rc.bottom, memDC, 0, 0, SRCCOPY);
    SelectObject(memDC, oldBmp);
    DeleteObject(memBmp);
    DeleteDC(memDC);

    EndPaint(hwnd, &ps);
    return 0;
}

void CandidateWindow::onLButtonDown(int x, int y) {
    int idx = (y - 5) / 22;
    if (idx >= 0 && idx < (int)candidates_.size()) {
        // 通知主服务提交
        // TODO: 通过 callback 通知 SamimeTextService
    }
}

// === SamimeTextService ===

SamimeTextService::SamimeTextService() : refCount_(1) {}

SamimeTextService::~SamimeTextService() {
    if (threadMgr_) {
        threadMgr_->Release();
    }
}

// === IUnknown ===

STDMETHODIMP SamimeTextService::QueryInterface(REFIID riid, void** ppv) {
    if (ppv == nullptr) return E_POINTER;
    if (riid == IID_IUnknown ||
        riid == IID_ITfTextInputProcessor ||
        riid == IID_ITfTextInputProcessorEx) {
        *ppv = (ITfTextInputProcessorEx*)this;
    } else if (riid == IID_ITfKeyEventSink) {
        *ppv = (ITfKeyEventSink*)this;
    } else if (riid == IID_ITfCompositionSink) {
        *ppv = (ITfCompositionSink*)this;
    } else if (riid == IID_ITfCandidateListUIElement) {
        *ppv = (ITfCandidateListUIElement*)this;
    } else if (riid == IID_ITfUIElement) {
        *ppv = (ITfUIElement*)this;
    } else {
        *ppv = nullptr;
        return E_NOINTERFACE;
    }
    AddRef();
    return S_OK;
}

STDMETHODIMP_(ULONG) SamimeTextService::AddRef() {
    return ++refCount_;
}

STDMETHODIMP_(ULONG) SamimeTextService::Release() {
    ULONG c = --refCount_;
    if (c == 0) {
        delete this;
    }
    return c;
}

// === ITfTextInputProcessor ===

STDMETHODIMP SamimeTextService::Activate(ITfThreadMgr* ptm, TfClientId tid) {
    return ActivateEx(ptm, tid, 0);
}

STDMETHODIMP SamimeTextService::ActivateEx(ITfThreadMgr* ptm, TfClientId tid, DWORD) {
    threadMgr_ = ptm;
    threadMgr_->AddRef();
    clientId_ = tid;

    // 注册按键事件 sink
    ITfKeystrokeMgr* keyMgr;
    if (SUCCEEDED(ptm->QueryInterface(IID_ITfKeystrokeMgr, (void**)&keyMgr))) {
        // 注册所有按键
        keyMgr->AdviseKeyEventSink(clientId_, (ITfKeyEventSink*)this, TRUE);

        // 注册保留键（可选）
        // 比如 Shift+Space 切换中英文

        keyMgr->Release();
    }

    // 创建候选窗口
    candWindow_.create(nullptr);

    // 连接 Go 引擎
    engine_.connect();

    OutputDebugStringW(L"[Samime] Activated");
    return S_OK;
}

STDMETHODIMP SamimeTextService::Deactivate() {
    if (threadMgr_) {
        ITfKeystrokeMgr* keyMgr;
        if (SUCCEEDED(threadMgr_->QueryInterface(IID_ITfKeystrokeMgr, (void**)&keyMgr))) {
            keyMgr->UnadviseKeyEventSink(clientId_);
            keyMgr->Release();
        }
        threadMgr_->Release();
        threadMgr_ = nullptr;
    }
    clientId_ = TF_CLIENTID_NULL;

    candWindow_.destroy();
    engine_.disconnect();

    OutputDebugStringW(L"[Samime] Deactivated");
    return S_OK;
}

// === ITfKeyEventSink ===

STDMETHODIMP SamimeTextService::OnSetFocus(BOOL) {
    return S_OK;
}

STDMETHODIMP SamimeTextService::OnTestKeyDown(ITfContext* pic, WPARAM wp, LPARAM lp, BOOL* pfEaten) {
    // 检查是否是我们处理的键
    if (wp >= 'A' && wp <= 'Z') { *pfEaten = TRUE; return S_OK; }
    if (wp >= '0' && wp <= '9') { *pfEaten = TRUE; return S_OK; }
    if (wp == VK_SPACE || wp == VK_RETURN || wp == VK_ESCAPE || wp == VK_BACK) {
        *pfEaten = TRUE;
    } else {
        *pfEaten = FALSE;
    }
    return S_OK;
}

STDMETHODIMP SamimeTextService::OnKeyDown(ITfContext* pic, WPARAM wp, LPARAM lp, BOOL* pfEaten) {
    *pfEaten = TRUE;

    // 字母键
    if (wp >= 'A' && wp <= 'Z') {
        // 处理大写锁定/Shift
        bool shifted = (GetKeyState(VK_SHIFT) & 0x80) != 0;
        wchar_t ch = (wchar_t)wp;
        if (!shifted) ch = ch + 32; // 转小写
        *pfEaten = handleCharacter(pic, ch);
        return S_OK;
    }

    // 数字键 1-9（选词）
    if (wp >= '1' && wp <= '9') {
        *pfEaten = handleDigit(pic, (int)(wp - '1'));
        return S_OK;
    }

    // 空格：选第一个候选
    if (wp == VK_SPACE) {
        *pfEaten = handleSpace(pic);
        return S_OK;
    }

    // 回车：提交当前候选
    if (wp == VK_RETURN) {
        *pfEaten = handleReturn(pic);
        return S_OK;
    }

    // ESC：清空
    if (wp == VK_ESCAPE) {
        *pfEaten = handleEscape(pic);
        return S_OK;
    }

    // 退格：删除最后一个字符
    if (wp == VK_BACK) {
        *pfEaten = handleBackspace(pic);
        return S_OK;
    }

    *pfEaten = FALSE;
    return S_OK;
}

STDMETHODIMP SamimeTextService::OnTestKeyUp(ITfContext*, WPARAM, LPARAM, BOOL* pfEaten) {
    *pfEaten = FALSE;
    return S_OK;
}

STDMETHODIMP SamimeTextService::OnKeyUp(ITfContext*, WPARAM, LPARAM, BOOL* pfEaten) {
    *pfEaten = FALSE;
    return S_OK;
}

STDMETHODIMP SamimeTextService::OnPreservedKey(ITfContext*, REFGUID, BOOL* pfEaten) {
    *pfEaten = FALSE;
    return S_OK;
}

// === ITfCompositionSink ===

STDMETHODIMP SamimeTextService::OnCompositionTerminated(TfEditCookie, ITfComposition*) {
    if (composition_) {
        composition_->Release();
        composition_ = nullptr;
    }
    preedit_.clear();
    candidates_.clear();
    candWindow_.hide();
    return S_OK;
}

// === ITfUIElement ===

STDMETHODIMP SamimeTextService::GetDescription(BSTR* pbstr) {
    if (pbstr == nullptr) return E_POINTER;
    *pbstr = SysAllocString(L"Samime Candidates");
    return S_OK;
}

STDMETHODIMP SamimeTextService::GetGUID(GUID* pguid) {
    if (pguid == nullptr) return E_POINTER;
    // {B2C3D4E5-F6A7-8901-BCDE-F12345678901}
    *pguid = { 0xB2C3D4E5, 0xF6A7, 0x8901, { 0xBC, 0xDE, 0xF1, 0x23, 0x45, 0x67, 0x89, 0x01 } };
    return S_OK;
}

STDMETHODIMP SamimeTextService::Show(BOOL bShow) {
    if (bShow) candWindow_.show();
    else candWindow_.hide();
    return S_OK;
}

STDMETHODIMP SamimeTextService::IsShown(BOOL* pbShow) {
    if (pbShow == nullptr) return E_POINTER;
    *pbShow = candWindow_.isVisible() ? TRUE : FALSE;
    return S_OK;
}

// === ITfCandidateListUIElement ===

STDMETHODIMP SamimeTextService::GetUpdatedFlags(DWORD* pdwFlags) {
    if (pdwFlags) *pdwFlags = TF_CLUIE_COUNT | TF_CLUIE_SELECTION | TF_CLUIE_STRING;
    return S_OK;
}

STDMETHODIMP SamimeTextService::GetDocumentMgr(ITfDocumentMgr** ppdim) {
    if (ppdim) *ppdim = nullptr;
    return E_NOTIMPL;
}

STDMETHODIMP SamimeTextService::GetCount(UINT* pcCandidateList) {
    if (pcCandidateList == nullptr) return E_POINTER;
    *pcCandidateList = (UINT)candidates_.size();
    return S_OK;
}

STDMETHODIMP SamimeTextService::GetSelection(UINT* puIndex) {
    if (puIndex == nullptr) return E_POINTER;
    *puIndex = (UINT)selectedIdx_;
    return S_OK;
}

STDMETHODIMP SamimeTextService::GetString(UINT uIndex, BSTR* pbstr) {
    if (pbstr == nullptr) return E_POINTER;
    if (uIndex >= candidates_.size()) return E_INVALIDARG;
    *pbstr = SysAllocString(candidates_[uIndex].word.c_str());
    return S_OK;
}

STDMETHODIMP SamimeTextService::GetPageIndex(UINT*, UINT, UINT*) {
    return E_NOTIMPL;
}

STDMETHODIMP SamimeTextService::SetPageIndex(UINT*, UINT) {
    return E_NOTIMPL;
}

STDMETHODIMP SamimeTextService::GetCurrentPage(UINT* puPage) {
    if (puPage) *puPage = 0;
    return S_OK;
}

// === 内部辅助方法 ===

bool SamimeTextService::handleCharacter(ITfContext* ctx, wchar_t ch) {
    preedit_ += ch;
    updatePreedit(ctx);
    return true;
}

bool SamimeTextService::handleBackspace(ITfContext* ctx) {
    if (preedit_.empty()) return false;
    preedit_.pop_back();
    if (preedit_.empty()) {
        reset();
    } else {
        updatePreedit(ctx);
    }
    return true;
}

bool SamimeTextService::handleReturn(ITfContext* ctx) {
    if (preedit_.empty()) return false;
    if (!candidates_.empty()) {
        return commitCandidate(ctx, selectedIdx_);
    }
    // 没候选，直接提交原始拼音
    insertText(ctx, preedit_);
    reset();
    return true;
}

bool SamimeTextService::handleEscape(ITfContext*) {
    if (preedit_.empty()) return false;
    reset();
    return true;
}

bool SamimeTextService::handleSpace(ITfContext* ctx) {
    if (preedit_.empty()) return false;
    if (!candidates_.empty()) {
        return commitCandidate(ctx, 0);
    }
    return handleReturn(ctx);
}

bool SamimeTextService::handleDigit(ITfContext* ctx, int idx) {
    if (preedit_.empty() || candidates_.empty()) return false;
    if (idx >= (int)candidates_.size()) return false;
    return commitCandidate(ctx, idx);
}

void SamimeTextService::updatePreedit(ITfContext* ctx) {
    if (preedit_.empty()) {
        if (composition_) {
            composition_->EndComposition(ctx);
            composition_->Release();
            composition_ = nullptr;
        }
        return;
    }

    // 调用 Go 引擎搜索
    candidates_ = engine_.search(preedit_);

    // 显示预编辑串
    setPreeditText(ctx, preedit_);

    // 显示候选窗
    if (!candidates_.empty()) {
        showCandidates(ctx);
    } else {
        candWindow_.hide();
    }
}

void SamimeTextService::showCandidates(ITfContext* ctx) {
    // 获取光标位置（简化处理：用屏幕坐标的鼠标位置）
    POINT pt;
    GetCursorPos(&pt);
    candWindow_.setPosition(pt.x, pt.y + 20);

    candWindow_.setCandidates(candidates_, 0);
    candWindow_.show();
}

bool SamimeTextService::commitCandidate(ITfContext* ctx, int idx) {
    if (idx < 0 || idx >= (int)candidates_.size()) return false;

    auto& c = candidates_[idx];
    engine_.commit(c.word, c.pinyin);

    // 插入文字到目标窗口
    insertText(ctx, c.word);

    reset();
    return true;
}

void SamimeTextService::reset() {
    preedit_.clear();
    candidates_.clear();
    selectedIdx_ = 0;
    engine_.reset();
    candWindow_.hide();
}

HRESULT SamimeTextService::insertText(ITfContext* ctx, const std::wstring& text) {
    if (text.empty()) return S_FALSE;

    ITfRange* range = nullptr;
    ITfEditSession* session = nullptr; // 简化处理，实际需要 edit session

    // 简化版：用 ITfInsertAtSelection
    ITfInsertAtSelection* insertAtSel;
    HRESULT hr = ctx->QueryInterface(IID_ITfInsertAtSelection, (void**)&insertAtSel);
    if (SUCCEEDED(hr)) {
        hr = insertAtSel->InsertTextAtSelection(TF_IAS_NOQUERY, text.c_str(),
                                                 (LONG)text.size(), &range);
        insertAtSel->Release();
    }
    if (range) range->Release();
    return hr;
}

HRESULT SamimeTextService::setPreeditText(ITfContext* ctx, const std::wstring& text) {
    // 完整实现需要 ITfContextComposition + ITfRange + IAnchor
    // 这里简化处理
    return S_OK;
}

// === SamimeClassFactory ===

STDMETHODIMP SamimeClassFactory::QueryInterface(REFIID riid, void** ppv) {
    if (ppv == nullptr) return E_POINTER;
    if (riid == IID_IUnknown || riid == IID_IClassFactory) {
        *ppv = this;
        AddRef();
        return S_OK;
    }
    *ppv = nullptr;
    return E_NOINTERFACE;
}

STDMETHODIMP SamimeClassFactory::CreateInstance(IUnknown* pUnkOuter, REFIID riid, void** ppv) {
    if (pUnkOuter) return CLASS_E_NOAGGREGATION;
    auto* obj = new SamimeTextService();
    HRESULT hr = obj->QueryInterface(riid, ppv);
    obj->Release();
    return hr;
}

} // namespace samime

// === DLL 导出 ===

static samime::SamimeClassFactory g_factory;

extern "C" {

HRESULT STDAPICALLTYPE DllGetClassObject(REFCLSID rclsid, REFIID riid, void** ppv) {
    if (rclsid != samime::CLSID_SamimeTextService) {
        return CLASS_E_CLASSNOTAVAILABLE;
    }
    return g_factory.QueryInterface(riid, ppv);
}

HRESULT STDAPICALLTYPE DllCanUnloadNow() {
    return S_FALSE;
}

HRESULT STDAPICALLTYPE DllRegisterServer() {
    // 简化：实际需要在 DllRegisterServer 中调用注册表写入函数
    // 详见 samime_reg.cpp
    return S_OK;
}

HRESULT STDAPICALLTYPE DllUnregisterServer() {
    return S_OK;
}

} // extern "C"
