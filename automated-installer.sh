#!/usr/bin/env bash

# Require script to be run as root
function super-user-check() {
  # This code checks to see if the script is running with root privileges.
  # If it is not, it will exit with an error message.
  if [ "${EUID}" -ne 0 ]; then
    echo "Error: You need to run this script as administrator."
    exit
  fi
}

# Check for root
super-user-check

# Get the current system information
function system-information() {
  # CURRENT_DISTRO is the ID of the current system
  # CURRENT_DISTRO_VERSION is the VERSION_ID of the current system
  if [ -f /etc/os-release ]; then
    # shellcheck source=/dev/null
    source /etc/os-release
    CURRENT_DISTRO=${ID}
    CURRENT_DISTRO_VERSION=${VERSION_ID}
  fi
}

# Get the current system information
system-information

function installing-system-requirements() {
  if { [ "${CURRENT_DISTRO}" == "ubuntu" ] || [ "${CURRENT_DISTRO}" == "debian" ] || [ "${CURRENT_DISTRO}" == "raspbian" ] || [ "${CURRENT_DISTRO}" == "pop" ] || [ "${CURRENT_DISTRO}" == "kali" ] || [ "${CURRENT_DISTRO}" == "linuxmint" ] || [ "${CURRENT_DISTRO}" == "neon" ] || [ "${CURRENT_DISTRO}" == "fedora" ] || [ "${CURRENT_DISTRO}" == "centos" ] || [ "${CURRENT_DISTRO}" == "rhel" ] || [ "${CURRENT_DISTRO}" == "almalinux" ] || [ "${CURRENT_DISTRO}" == "rocky" ] || [ "${CURRENT_DISTRO}" == "arch" ] || [ "${CURRENT_DISTRO}" == "archarm" ] || [ "${CURRENT_DISTRO}" == "manjaro" ] || [ "${CURRENT_DISTRO}" == "alpine" ] || [ "${CURRENT_DISTRO}" == "freebsd" ] || [ "${CURRENT_DISTRO}" == "ol" ]; }; then
    if { [ ! -x "$(command -v cut)" ] || [ ! -x "$(command -v ffmpeg)" ] || [ ! -x "$(command -v curl)" ] || [ ! -x "$(command -v vlc)" ]; }; then
      if { [ "${CURRENT_DISTRO}" == "ubuntu" ] || [ "${CURRENT_DISTRO}" == "debian" ] || [ "${CURRENT_DISTRO}" == "raspbian" ] || [ "${CURRENT_DISTRO}" == "pop" ] || [ "${CURRENT_DISTRO}" == "kali" ] || [ "${CURRENT_DISTRO}" == "linuxmint" ] || [ "${CURRENT_DISTRO}" == "neon" ]; }; then
        apt-get update
        apt-get install coreutils ffmpeg curl vlc -y
      elif { [ "${CURRENT_DISTRO}" == "fedora" ] || [ "${CURRENT_DISTRO}" == "centos" ] || [ "${CURRENT_DISTRO}" == "rhel" ] || [ "${CURRENT_DISTRO}" == "almalinux" ] || [ "${CURRENT_DISTRO}" == "rocky" ]; }; then
        yum check-update
        yum install coreutils ffmpeg curl vlc -y
      elif { [ "${CURRENT_DISTRO}" == "arch" ] || [ "${CURRENT_DISTRO}" == "archarm" ] || [ "${CURRENT_DISTRO}" == "manjaro" ]; }; then
        pacman -Sy --noconfirm archlinux-keyring
        pacman -Su --noconfirm --needed coreutils ffmpeg curl vlc
      elif [ "${CURRENT_DISTRO}" == "alpine" ]; then
        apk update
        apk add coreutils ffmpeg curl vlc
      elif [ "${CURRENT_DISTRO}" == "freebsd" ]; then
        pkg update
        pkg install coreutils ffmpeg curl vlc
      elif [ "${CURRENT_DISTRO}" == "ol" ]; then
        yum check-update
        yum install coreutils ffmpeg curl vlc -y
      fi
    fi
  else
    echo "Error: ${CURRENT_DISTRO} ${CURRENT_DISTRO_VERSION} is not supported."
    exit
  fi
}

# check for requirements
installing-system-requirements

# Only allow certain init systems
function check-current-init-system() {
  # This code checks if the current init system is systemd or sysvinit
  # If it is neither, the script exits
  CURRENT_INIT_SYSTEM=$(ps --no-headers -o comm 1)
  case ${CURRENT_INIT_SYSTEM} in
  *"systemd"* | *"init"*) ;;
  *)
    echo "${CURRENT_INIT_SYSTEM} init is not supported (yet)."
    exit
    ;;
  esac
}

# Check if the current init system is supported
check-current-init-system

# Check if there are enough space to continue with the installation.
function check-disk-space() {
  # Checks to see if there is more than 1 GB of free space on the drive
  # where the user is installing to. If there is not, it will exit the
  # script.
  FREE_SPACE_ON_DRIVE_IN_MB=$(df -m / | tr --squeeze-repeats " " | tail -n1 | cut --delimiter=" " --fields=4)
  if [ "${FREE_SPACE_ON_DRIVE_IN_MB}" -le 1024 ]; then
    echo "Error: More than 1 GB of free space is needed to install everything."
    exit
  fi
}

# Check if there is enough disk space
check-disk-space

# Global variables
RTSP_SIMPLE_SERVER_PATH="/etc/rtsp-simple-server"
RTSP_SIMPLE_SERVICE_APPLICATION="${RTSP_SIMPLE_SERVER_PATH}/rtsp-simple-server"
RTSP_SIMPLE_SERVER_SERVICE="/etc/systemd/system/rtsp-simple-server.service"
LATEST_RELEASE=$(curl -s https://api.github.com/repos/aler9/rtsp-simple-server/releases/latest | grep browser_download_url | cut -d'"' -f4 | grep $(dpkg --print-architecture) | grep linux)
LASTEST_FILE_NAME=$(echo "${LATEST_RELEASE}" | cut --delimiter="/" --fields=9)

# Check if the rtsp-simple-server directory dosent exists
if [ ! -d "${RTSP_SIMPLE_SERVER_PATH}" ]; then

  # Download the latest release
  function download-latest-release() {
    # This code downloads the latest release
    # The latest release is stored in the variable LATEST_RELEASE
    # The latest release is downloaded to /tmp/
    curl -L "${LATEST_RELEASE}" -o /tmp/${LASTEST_FILE_NAME}
    mkdir -p ${RTSP_SIMPLE_SERVER_PATH}
    tar -xvf /tmp/${LASTEST_FILE_NAME} -C ${RTSP_SIMPLE_SERVER_PATH}
  }

  # Download the latest release
  download-latest-release

  # Create the service file
  function create-service-file() {
    # This code creates the service file
    # The service file is stored in /etc/systemd/system/rtsp-simple-server.service
    echo "[Unit]
Wants=network.target
[Service]
ExecStart=${RTSP_SIMPLE_SERVICE_APPLICATION}
[Install]
WantedBy=multi-user.target" >${RTSP_SIMPLE_SERVER_SERVICE}
    if [[ "${CURRENT_INIT_SYSTEM}" == *"systemd"* ]]; then
      systemctl daemon-reload
      systemctl enable rtsp-simple-server
      systemctl restart rtsp-simple-server
    elif [[ "${CURRENT_INIT_SYSTEM}" == *"init"* ]]; then
      service rtsp-simple-server restart
    fi
  }

  # Create the service file
  create-service-file

else

  # Ask the user if they want to uninstall the rtsp-simple-server
  function uninstall-rtsp-simple-server() {
    # This code asks the user if they want to uninstall the rtsp-simple-server
    # If the user answers yes, the rtsp-simple-server is uninstalled
    # If the user answers no, the script exits
    read -r -p "Do you want to uninstall the rtsp-simple-server? [y/N] " response
    case "${response}" in
    [yY][eE][sS] | [yY])
      echo "Uninstalling the rtsp-simple-server..."
      if [[ "${CURRENT_INIT_SYSTEM}" == *"systemd"* ]]; then
        systemctl stop rtsp-simple-server
        systemctl disable rtsp-simple-server
      elif [[ "${CURRENT_INIT_SYSTEM}" == *"init"* ]]; then
        service rtsp-simple-server stop
      fi
      rm -rf ${RTSP_SIMPLE_SERVER_PATH}
      rm -rf ${RTSP_SIMPLE_SERVER_SERVICE}
      systemctl daemon-reload
      echo "Uninstalled the rtsp-simple-server."
      exit
      ;;
    *)
      echo "Exiting..."
      exit
      ;;
    esac
  }

  # Uninstall the rtsp-simple-server
  uninstall-rtsp-simple-server

fi
