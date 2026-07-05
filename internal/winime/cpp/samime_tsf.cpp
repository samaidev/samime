// GoIME TSF Proxy - C++ 薄壳
// 通过 TSF COM 接口接收按键事件，转发给 Go 引擎（命名管道）
//
// 编译:
//   cl /EHsc /LD samime_tsf.cpp samime_tsf.def
//   或 MinGW: g++ -shared -o samime_tsf.dll samime_tsf.cpp -lole32 -loleaut32
//
// 安装:
//   将 samime_tsf.dll 复制到 C:\Program Files\SamIME\
//   regsvr32 samime_tsf.dll
//
// 本文件是简化骨架，仅展示关键 COM 接口实现思路
// 完整 TSF 实现需要 ~2000 行 C++ 代码，参考 Microsoft TSF 示例：
//   https://github.com/microsoft/Windows-classic-samples/tree/main/Samples/Win7Samples/winui/input/tsf

#define WIN32_LEAN_AND_MEAN
#include <windows.h>
#include <ole2.h>
#include <olectl.h>
#include <msctf.h>
#include <atomic>
#include <string>
#include <thread>
#include <winsock2.h>
#include <ws2tcpip.h>

#pragma comment(lib, "ole32.lib")
#pragma comment(lib, "oleaut32.lib")
#pragma comment(lib, "msctf.lib")
#pragma comment(lib, "ws2_32.lib")

// === Go 引擎通信 ===
// 通过命名管道 \\.\pipe\goime 与 Go 服务通信
// 协议：JSON over line-delimited

class GoEngineClient {
public:
    GoEngineClient() : sock_(INVALID_SOCKET) {
        WSADATA wsaData;
        WSAStartup(MAKEWORD(2, 2), &wsaData);
    }

    ~GoEngineClient() {
        disconnect();
        WSACleanup();
    }

    bool connect() {
        // 命名管道：用 CreateFile
        // 这里简化为 TCP（开发模式）
        sock_ = socket(AF_INET, SOCK_STREAM, IPPROTO_TCP);
        if (sock_ == INVALID_SOCKET) return false;

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

    void disconnect() {
        if (sock_ != INVALID_SOCKET) {
            closesocket(sock_);
            sock_ = INVALID_SOCKET;
        }
    }

    // 发送搜索请求，返回候选词列表
    std::string search(const std::wstring& preedit) {
        if (!ensureConnected()) return "{}";
        std::string preeditUtf8 = wideToUtf8(preedit);
        std::string req = "{\"method\":\"search\",\"preedit\":\"" + escapeJson(preeditUtf8) + "\"}\n";
        send(sock_, req.c_str(), (int)req.size(), 0);
        return readResponse();
    }

    // 提交选定的词
    void commit(const std::wstring& word, const std::wstring& pinyin) {
        if (!ensureConnected()) return;
        std::string wordUtf8 = wideToUtf8(word);
        std::string pyUtf8 = wideToUtf8(pinyin);
        std::string req = "{\"method\":\"commit\",\"word\":\"" + escapeJson(wordUtf8) +
                          "\",\"pinyin\":\"" + escapeJson(pyUtf8) + "\"}\n";
        send(sock_, req.c_str(), (int)req.size(), 0);
        readResponse();
    }

    void reset() {
        if (!ensureConnected()) return;
        send(sock_, "{\"method\":\"reset\"}\n", 20, 0);
        readResponse();
    }

private:
    bool ensureConnected() {
        if (sock_ != INVALID_SOCKET) return true;
        return connect();
    }

    std::string readResponse() {
        char buf[65536];
        int total = 0;
        while (total < (int)sizeof(buf) - 1) {
            int n = recv(sock_, buf + total, (int)sizeof(buf) - 1 - total, 0);
            if (n <= 0) break;
            total += n;
            buf[total] = '\0';
            if (strchr(buf, '\n')) break;
        }
        buf[total] = '\0';
        return std::string(buf);
    }

    static std::string wideToUtf8(const std::wstring& w) {
        if (w.empty()) return "";
        int len = WideCharToMultiByte(CP_UTF8, 0, w.c_str(), (int)w.size(), nullptr, 0, nullptr, nullptr);
        std::string s(len, '\0');
        WideCharToMultiByte(CP_UTF8, 0, w.c_str(), (int)w.size(), &s[0], len, nullptr, nullptr);
        return s;
    }

    static std::string escapeJson(const std::string& s) {
        std::string out;
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
                        sprintf(buf, "\\u%04x", c);
                        out += buf;
                    } else {
                        out += c;
                    }
            }
        }
        return out;
    }

    SOCKET sock_;
};

// === TSF TextService 实现（骨架）===
// 完整实现需继承 ITfTextInputProcessorEx + 多个接口

class CSamimeTextService : public ITfTextInputProcessorEx {
public:
    CSamimeTextService() : refCount_(1), client_() {}

    // IUnknown
    STDMETHODIMP QueryInterface(REFIID riid, void** ppv) override {
        if (riid == IID_IUnknown || riid == IID_ITfTextInputProcessor ||
            riid == IID_ITfTextInputProcessorEx) {
            *ppv = this;
            AddRef();
            return S_OK;
        }
        *ppv = nullptr;
        return E_NOINTERFACE;
    }
    STDMETHODIMP_(ULONG) AddRef() override {
        return ++refCount_;
    }
    STDMETHODIMP_(ULONG) Release() override {
        if (--refCount_ == 0) {
            delete this;
            return 0;
        }
        return refCount_;
    }

    // ITfTextInputProcessor
    STDMETHODIMP Activate(ITfThreadMgr* ptm, TfClientId tid) override {
        // 注册按键事件监听
        // 启动 Go 引擎客户端
        client_.connect();
        OutputDebugStringW(L"[SamIME] Activated");
        return S_OK;
    }

    STDMETHODIMP Deactivate() override {
        client_.disconnect();
        OutputDebugStringW(L"[SamIME] Deactivated");
        return S_OK;
    }

    // ITfTextInputProcessorEx
    STDMETHODIMP ActivateEx(ITfThreadMgr* ptm, TfClientId tid, DWORD dwFlags) override {
        return Activate(ptm, tid);
    }

    // 按键事件处理（伪代码）
    // 实际 TSF 中需要实现 ITfKeyEventSink
    HRESULT OnKeyDown(WPARAM wParam, LPARAM lParam, BOOL* pfEaten) {
        // 1. 把按键加入预编辑缓冲区
        // 2. 调用 client_.search(preedit)
        // 3. 显示候选词列表
        // 4. 用户选词时调用 client_.commit(word, pinyin)
        // 5. 通过 ITfInsertAtSelection 把文字插入到目标窗口
        *pfEaten = TRUE;
        return S_OK;
    }

private:
    std::atomic<ULONG> refCount_;
    GoEngineClient client_;
};

// === Class Factory ===
class CSamimeClassFactory : public IClassFactory {
public:
    STDMETHODIMP QueryInterface(REFIID riid, void** ppv) override {
        if (riid == IID_IUnknown || riid == IID_IClassFactory) {
            *ppv = this;
            AddRef();
            return S_OK;
        }
        *ppv = nullptr;
        return E_NOINTERFACE;
    }
    STDMETHODIMP_(ULONG) AddRef() override { return 1; }
    STDMETHODIMP_(ULONG) Release() override { return 1; }

    STDMETHODIMP CreateInstance(IUnknown* pUnkOuter, REFIID riid, void** ppv) override {
        if (pUnkOuter) return CLASS_E_NOAGGREGATION;
        auto* obj = new CSamimeTextService();
        HRESULT hr = obj->QueryInterface(riid, ppv);
        obj->Release();
        return hr;
    }

    STDMETHODIMP LockServer(BOOL fLock) override { return S_OK; }
};

// === DLL 导出 ===
CSamimeClassFactory g_factory;

extern "C" {
    HRESULT STDAPICALLTYPE DllGetClassObject(REFCLSID rclsid, REFIID riid, void** ppv) {
        if (rclsid != CLSID_SamimeTextService) return CLASS_E_CLASSNOTAVAILABLE;
        return g_factory.QueryInterface(riid, ppv);
    }

    HRESULT STDAPICALLTYPE DllCanUnloadNow() {
        return S_FALSE;
    }

    HRESULT STDAPICALLTYPE DllRegisterServer() {
        // 注册 CLSID, 注册为 TSF 文本服务
        // 完整实现参考 TSF SDK 示例
        return S_OK;
    }

    HRESULT STDAPICALLTYPE DllUnregisterServer() {
        return S_OK;
    }
}

// CLSID 需要唯一生成，这里用占位符
const CLSID CLSID_SamimeTextService = {
    0x12345678, 0x1234, 0x1234, {0x12, 0x34, 0x56, 0x78, 0x9a, 0xbc, 0xde, 0xf0}
};
