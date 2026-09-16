Name:           zealish-wa
Version:        1.0.0
Release:        1%{?dist}
Summary:        Native WhatsApp Web client for Linux

License:        Proprietary
URL:            https://github.com/zealish/zealish-whatsapp
Source0:        %{name}-%{version}.tar.gz

BuildRequires:  golang >= 1.25
BuildRequires:  gcc
BuildRequires:  xdotool
Requires:       google-chrome-stable
Requires:       xdotool
Requires:       xdg-utils

%description
Desktop client for WhatsApp Web built on Go with a dedicated Chromium app window.
Chromium provides the WebRTC engine required for WhatsApp voice and video calls.

%prep
%autosetup

%build
export CGO_ENABLED=1
go build -trimpath -ldflags "-s -w" -o bin/%{name} ./cmd/app

%install
install -Dm0755 bin/%{name} %{buildroot}%{_bindir}/%{name}
install -Dm0644 assets/com.zealish.WhatsApp.desktop \
    %{buildroot}%{_datadir}/applications/com.zealish.WhatsApp.desktop
install -Dm0644 assets/icon.svg \
    %{buildroot}%{_datadir}/icons/hicolor/scalable/apps/com.zealish.WhatsApp.svg

%files
%{_bindir}/%{name}
%{_datadir}/applications/com.zealish.WhatsApp.desktop
%{_datadir}/icons/hicolor/scalable/apps/com.zealish.WhatsApp.svg

%changelog
* Sat Sep 13 2026 Zealish <dev@zealish.local> - 1.0.0-1
- Initial release
