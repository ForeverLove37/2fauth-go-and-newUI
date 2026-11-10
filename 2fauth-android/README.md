# 2FAuth Android

原生 Android 客户端（Kotlin + Jetpack Compose），连接你自己的 2FAuth 服务器。配色、玻璃卡片、服务识别色、倒计时环和“2F”标志与网页面板一致。

## 和网页端一样的地方

- 同一套深色主题和卡片样式，每个服务的颜色与网页上相同
- 验证码默认隐藏，点按卡片显示 5 秒；“显示全部”、“显示下一个验证码”、“显示图标”是服务器上的偏好，两边同步
- 分组筛选、置顶、移到分组、删除；添加账户（二维码、otpauth 链接、Google Authenticator 等应用的导出、手动输入）

## 为手机做的调整

- **点按即复制**：复制时带触感反馈；剪贴板内容标记为敏感（系统剪贴板预览只显示圆点），默认 30 秒后自动清除
- **应用锁**：指纹、面容或手机锁屏密码；可设置离开后立即、30 秒、1/5/15 分钟锁定
- **禁止截屏**（默认开启）：截屏、录屏和最近任务里都看不到验证码
- **密钥不落地**：手机上不保存任何 TOTP 密钥，验证码由服务器计算；登录信息用 Android Keystore 的密钥加密保存，并排除在系统备份之外
- **自己的令牌**：用密码或 SSO 登录后，应用会为这台手机创建一个个人访问令牌并立即结束网页会话，因此不会两小时闲置就掉线；在网页端「设置 → 安全 → 个人访问令牌」里可以随时吊销，退出登录时自动吊销
- **自签名证书**：内网实例使用自签名证书时，应用显示证书的 SHA-256 指纹，确认后只信任这一张证书；证书一旦变化就拒绝连接
- 验证码分两段显示（`123 456`），最后 10 秒倒计时环变黄、5 秒变红；剩余不足 5 秒时复制会提示
- 下拉刷新、搜索、置顶账户排在最前；平板和横屏自动多列
- 其他应用或网页中的 `otpauth://`、`otpauth-migration://` 链接可以直接用本应用打开
- 拍照扫码不需要相机权限（由系统相机拍摄），照片识别后立即删除
- 中文、英文界面

## 登录方式

| 方式 | 说明 |
|---|---|
| 邮箱 + 密码 | 普通登录；成功后换成本机令牌 |
| SSO（OIDC） | 在应用内的网页视图中完成，回到面板后换成本机令牌 |
| 个人访问令牌 | 粘贴在网页端创建的令牌 |

服务器开启“仅 SSO 登录”且不允许个人访问令牌时，应用会保留会话 Cookie，闲置超过服务器的会话时限（默认 2 小时）后需要重新登录。WebAuthn（通行密钥）登录不支持。

服务器的 `ALLOWED_CLIENT_IPS` 同样限制手机：手机所在网络的地址不在名单中时，应用会提示“这台服务器不接受来自当前网络的连接”。

## 构建

需要 JDK 17 和 Android SDK（platform 35、build-tools 35）。

```bash
cd 2fauth-android
echo "sdk.dir=$ANDROID_HOME" > local.properties
./gradlew assembleDebug       # app/build/outputs/apk/debug/app-debug.apk，包名 app.twofauth.android.debug
./gradlew assembleRelease     # 需要 keystore.properties
```

正式包的签名信息写在 `keystore.properties`（不进仓库）：

```properties
storeFile=/path/to/2fauth-release.jks
storePassword=...
keyAlias=2fauth
keyPassword=...
```

以后发布更新必须使用同一个签名密钥，否则手机上无法覆盖安装。请备份好密钥文件和密码。

最低支持 Android 8.0（API 26），目标 Android 15（API 35）。
