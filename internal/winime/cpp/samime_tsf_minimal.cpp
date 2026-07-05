// samime_tsf_minimal.cpp - 最简版 TSF DLL
// 只实现 COM 注册（DllRegisterServer），让输入法出现在系统列表
// 完整 TSF 实现见 samime_tsf.cpp（需进一步调试编译）
//
// 编译: cl /EHsc /LD samime_tsf_minimal.cpp /link ole32.lib oleaut32.lib shlwapi.lib /DEF:samime_tsf.def

#define WIN32_LEAN_AND_MEAN
#define _UNICODE
#define UNICODE
#include <windows.h>
#include <ole2.h>
#include <olectl.h>
#include <objbase.h>
#include <shlwapi.h>
#include <strsafe.h>
#include <tchar.h>

// {A1B2C3D4-E5F6-7890-ABCD-EF1234567890}
static const CLSID CLSID_SamimeTextService = {
    0xA1B2C3D4, 0xE5F6, 0x7890,
    {0xAB, 0xCD, 0xEF, 0x12, 0x34, 0x56, 0x78, 0x90}
};

// 简单的 IClassFactory
class CSamimeClassFactory : public IClassFactory {
public:
    STDMETHODIMP QueryInterface(REFIID riid, void** ppv) {
        if (riid == IID_IUnknown || riid == IID_IClassFactory) {
            *ppv = this;
            AddRef();
            return S_OK;
        }
        *ppv = nullptr;
        return E_NOINTERFACE;
    }
    STDMETHODIMP_(ULONG) AddRef() { return 1; }
    STDMETHODIMP_(ULONG) Release() { return 1; }
    STDMETHODIMP CreateInstance(IUnknown* pUnkOuter, REFIID riid, void** ppv) {
        if (pUnkOuter) return CLASS_E_NOAGGREGATION;
        // 最简版：不创建实际对象
        *ppv = nullptr;
        return E_NOTIMPL;
    }
    STDMETHODIMP LockServer(BOOL fLock) { return S_OK; }
};

static CSamimeClassFactory g_factory;

// 注册表路径
static const TCHAR* kClsidPath = _T("CLSID\\{A1B2C3D4-E5F6-7890-ABCD-EF1234567890}");
static const TCHAR* kClsidInproc = _T("CLSID\\{A1B2C3D4-E5F6-7890-ABCD-EF1234567890}\\InprocServer32");

static BOOL setRegString(HKEY root, const TCHAR* path, const TCHAR* name, const TCHAR* value) {
    HKEY key;
    if (RegCreateKeyEx(root, path, 0, nullptr, 0, KEY_WRITE, nullptr, &key, nullptr) != ERROR_SUCCESS)
        return FALSE;
    LONG result = RegSetValueEx(key, name, 0, REG_SZ, (BYTE*)value,
                                 (DWORD)(_tcslen(value) + 1) * sizeof(TCHAR));
    RegCloseKey(key);
    return result == ERROR_SUCCESS;
}

extern "C" {

HRESULT STDAPICALLTYPE DllGetClassObject(REFCLSID rclsid, REFIID riid, void** ppv) {
    if (rclsid != CLSID_SamimeTextService)
        return CLASS_E_CLASSNOTAVAILABLE;
    return g_factory.QueryInterface(riid, ppv);
}

HRESULT STDAPICALLTYPE DllCanUnloadNow() {
    return S_FALSE;
}

HRESULT STDAPICALLTYPE DllRegisterServer() {
    TCHAR dllPath[MAX_PATH];
    GetModuleFileName(GetModuleHandle(_T("samime_tsf")), dllPath, MAX_PATH);

    // 注册 COM 服务器
    setRegString(HKEY_CLASSES_ROOT, kClsidPath, nullptr, _T("Samime TextService"));
    setRegString(HKEY_CLASSES_ROOT, kClsidInproc, nullptr, dllPath);
    setRegString(HKEY_CLASSES_ROOT, kClsidInproc, _T("ThreadingModel"), _T("Apartment"));

    // 注册 TSF 文本服务
    setRegString(HKEY_LOCAL_MACHINE,
        _T("Software\\Microsoft\\CTF\\TIP\\{A1B2C3D4-E5F6-7890-ABCD-EF1234567890}"),
        _T("Description"), _T("Samime Chinese Input Method"));
    setRegString(HKEY_LOCAL_MACHINE,
        _T("Software\\Microsoft\\CTF\\TIP\\{A1B2C3D4-E5F6-7890-ABCD-EF1234567890}"),
        _T("File"), dllPath);

    // 注册 Language Profile
    setRegString(HKEY_LOCAL_MACHINE,
        _T("Software\\Microsoft\\CTF\\TIP\\{A1B2C3D4-E5F6-7890-ABCD-EF1234567890}\\LanguageProfile\\0x00000804"),
        _T("Description"), _T("Samime Chinese Input Method"));
    setRegString(HKEY_LOCAL_MACHINE,
        _T("Software\\Microsoft\\CTF\\TIP\\{A1B2C3D4-E5F6-7890-ABCD-EF1234567890}\\LanguageProfile\\0x00000804"),
        _T("Name"), _T("Samime"));
    setRegString(HKEY_LOCAL_MACHINE,
        _T("Software\\Microsoft\\CTF\\TIP\\{A1B2C3D4-E5F6-7890-ABCD-EF1234567890}\\LanguageProfile\\0x00000804"),
        _T("Icon"), dllPath);
    setRegString(HKEY_LOCAL_MACHINE,
        _T("Software\\Microsoft\\CTF\\TIP\\{A1B2C3D4-E5F6-7890-ABCD-EF1234567890}\\LanguageProfile\\0x00000804"),
        _T("TipFile"), dllPath);

    return S_OK;
}

HRESULT STDAPICALLTYPE DllUnregisterServer() {
    SHDeleteKey(HKEY_CLASSES_ROOT, kClsidPath);
    SHDeleteKey(HKEY_LOCAL_MACHINE,
        _T("Software\\Microsoft\\CTF\\TIP\\{A1B2C3D4-E5F6-7890-ABCD-EF1234567890}"));
    return S_OK;
}

} // extern "C"
