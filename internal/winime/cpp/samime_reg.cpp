// samime_reg.cpp - TSF 注册表写入
#define _UNICODE
#define UNICODE

#include <tchar.h>
#include "samime_tsf.h"
#include <strsafe.h>

namespace samime {

extern const CLSID CLSID_SamimeTextService;
extern const TCHAR* const SAMIME_SERVICE_DESC;
extern const TCHAR* const SAMIME_SERVICE_NAME;

// 文本服务 Profile GUID
// {C3D4E5F6-A7B8-9012-CDEF-23456789012}
static const GUID SAMIME_PROFILE_GUID = {
    0xC3D4E5F6, 0xA7B8, 0x9012,
    {0xCD, 0xEF, 0x23, 0x45, 0x67, 0x89, 0x01, 0x2}
};

// 语言 GUID (简体中文)
static const GUID GUID_LMIC_SAMIME_ZH_CN = {
    0x81D4E9C9, 0x1D9B, 0x4C2C,
    {0xA6, 0xE6, 0x49, 0x65, 0x98, 0x6E, 0x4D, 0x6F}
};

static const TCHAR* const kClsidPath =
    _T("CLSID\\{A1B2C3D4-E5F6-7890-ABCD-EF1234567890}");
static const TCHAR* const kClsidInproc =
    _T("CLSID\\{A1B2C3D4-E5F6-7890-ABCD-EF1234567890}\\InprocServer32");
static const TCHAR* const kTsfPath =
    _T("Software\\Microsoft\\CTF\\TIP\\{A1B2C3D4-E5F6-7890-ABCD-EF1234567890}");
static const TCHAR* const kProfilePath =
    _T("Software\\Microsoft\\CTF\\TIP\\{A1B2C3D4-E5F6-7890-ABCD-EF1234567890}\\LanguageProfile");
static const TCHAR* const kCatPath =
    _T("Component Categories\\{534C48D1-F021-4F5C-B59B-4F0D2F75C3CC}");

// 写入字符串到注册表
static BOOL setRegString(HKEY root, const TCHAR* path, const TCHAR* name, const TCHAR* value) {
    HKEY key;
    if (RegCreateKeyEx(root, path, 0, nullptr, 0, KEY_WRITE, nullptr, &key, nullptr) != ERROR_SUCCESS) {
        return FALSE;
    }
    LONG result = RegSetValueEx(key, name, 0, REG_SZ, (BYTE*)value,
                                 (DWORD)(_tcslen(value) + 1) * sizeof(TCHAR));
    RegCloseKey(key);
    return result == ERROR_SUCCESS;
}

static BOOL setRegDword(HKEY root, const TCHAR* path, const TCHAR* name, DWORD value) {
    HKEY key;
    if (RegCreateKeyEx(root, path, 0, nullptr, 0, KEY_WRITE, nullptr, &key, nullptr) != ERROR_SUCCESS) {
        return FALSE;
    }
    LONG result = RegSetValueEx(key, name, 0, REG_DWORD, (BYTE*)&value, sizeof(value));
    RegCloseKey(key);
    return result == ERROR_SUCCESS;
}

static BOOL deleteRegTree(HKEY root, const TCHAR* path) {
    return SHDeleteKey(root, path) == ERROR_SUCCESS;
}

// 注册
HRESULT registerServer() {
    TCHAR dllPath[MAX_PATH];
    GetModuleFileName(GetModuleHandle(_T("samime_tsf.dll")), dllPath, MAX_PATH);

    // 1. 注册 COM 服务器
    setRegString(HKEY_CLASSES_ROOT, kClsidPath, nullptr, _T("Samime TextService"));
    setRegString(HKEY_CLASSES_ROOT, kClsidInproc, nullptr, dllPath);
    setRegString(HKEY_CLASSES_ROOT, kClsidInproc, _T("ThreadingModel"), _T("Apartment"));

    // 2. 注册 TSF 文本服务
    setRegString(HKEY_LOCAL_MACHINE, kTsfPath, _T("Description"), SAMIME_SERVICE_DESC);
    setRegString(HKEY_LOCAL_MACHINE, kTsfPath, _T("File"), dllPath);
    setRegString(HKEY_LOCAL_MACHINE, kTsfPath, _T("TextService"), SAMIME_SERVICE_NAME);

    // 3. 注册 Language Profile
    TCHAR profilePath[MAX_PATH];
    StringCchCopy(profilePath, MAX_PATH, kProfilePath);
    StringCchCat(profilePath, MAX_PATH, _T("\\0x00000804"));
    setRegString(HKEY_LOCAL_MACHINE, profilePath, _T("Description"), SAMIME_SERVICE_DESC);
    setRegString(HKEY_LOCAL_MACHINE, profilePath, _T("Icon"), dllPath);
    setRegString(HKEY_LOCAL_MACHINE, profilePath, _T("Name"), SAMIME_SERVICE_NAME);
    setRegString(HKEY_LOCAL_MACHINE, profilePath, _T("TipFile"), dllPath);

    // 4. 注册类别 (TSF TextService Category)
    setRegString(HKEY_CLASSES_ROOT,
        _T("CLSID\\{A1B2C3D4-E5F6-7890-ABCD-EF1234567890}\\Implemented Categories\\{534C48D1-F021-4F5C-B59B-4F0D2F75C3CC}"),
        nullptr, nullptr);

    return S_OK;
}

// 注销
HRESULT unregisterServer() {
    deleteRegTree(HKEY_CLASSES_ROOT, kClsidPath);
    deleteRegTree(HKEY_LOCAL_MACHINE, kTsfPath);
    return S_OK;
}

} // namespace samime

// 重写 DllRegisterServer / DllUnregisterServer
#undef DllRegisterServer
#undef DllUnregisterServer

extern "C" {
    HRESULT STDAPICALLTYPE DllRegisterServer() {
        return samime::registerServer();
    }
    HRESULT STDAPICALLTYPE DllUnregisterServer() {
        return samime::unregisterServer();
    }
}
