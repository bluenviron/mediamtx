using System;
using System.Diagnostics;
using System.IO;
using System.ServiceProcess;
using System.Text;
using System.Windows.Forms;

namespace PiscadaMtx
{
    public partial class Form1 : Form
    {   
        private FileSystemWatcher logWatcher;
        private string serviceName = "mtx";
        private string SERVICE_EXE_FILE = "mtx_service.exe";
        private string MEDIAMTX_CONFIG_FILE = "mediamtx.yml";
        private bool READ_WHOLE_FILE = false;
        private string LOG_FOLDER_PATH = Path.Combine(AppDomain.CurrentDomain.BaseDirectory, "logs");
        const int NUMER_OF_LINES = 500;


        public Form1()
        {
            InitializeComponent();
            InitializeLogWatcher();
            UpdateServiceStatus();
        }

        private void InitializeLogWatcher()
        {
            logWatcher = new FileSystemWatcher();
            logWatcher.Path = Path.Combine(AppDomain.CurrentDomain.BaseDirectory, "logs");
            logWatcher.Filter = GetLogFileName();
            logWatcher.NotifyFilter = NotifyFilters.LastWrite;
            logWatcher.Changed += new FileSystemEventHandler(OnLogChanged);
            logWatcher.EnableRaisingEvents = true;
        }

        private void OnLogChanged(object sender, FileSystemEventArgs e)
        {
            if (InvokeRequired)
            {
                this.Invoke(new Action(() => OnLogChanged(sender, e)));
                return;
            }

            RefreshLog();
        }

        private string GetLogFileName()
        {
            // Example file output
            //mtx_service_2024_08_05.err.log

            DateTime today = DateTime.Today;
            string formattedDate = today.ToString("yyyy_MM_dd");
            string logOutFileName = $"mtx_service_{formattedDate}.out.log";
            return logOutFileName;
        }

        private string GetOutLogFilePath()
        {
            return Path.Combine(LOG_FOLDER_PATH, GetLogFileName());
        }

        private void RefreshLog()
        {
            string logPath = GetOutLogFilePath();
            if (File.Exists(logPath))
            {
                string logContent = ReadLogFile(logPath);
                DisplayFormattedLogContent(logContent);
            }
        }

        private string ReadLogFile(string logPath)
        {
            try
            {
                using (FileStream fileStream = new FileStream(logPath, FileMode.Open, FileAccess.Read, FileShare.ReadWrite))
                using (StreamReader reader = new StreamReader(fileStream, Encoding.UTF8))
                {
                    if (READ_WHOLE_FILE)
                    {
                        string content = reader.ReadToEnd();
                        return NormalizeLineEndings(content);
                    }

                    // Read the tail of the file, starting from the last 1000 lines

                    var lines = new List<string>();
                    var buffer = new char[1024];
                    int bytesRead;
                    long position = fileStream.Length;

                    while (position > 0 && lines.Count < NUMER_OF_LINES)
                    {
                        int readLength = (int)Math.Min(buffer.Length, position);
                        fileStream.Seek(position - readLength, SeekOrigin.Begin);
                        bytesRead = reader.ReadBlock(buffer, 0, readLength);

                        for (int i = bytesRead - 1; i >= 0; i--)
                        {
                            if (buffer[i] == '\n')
                            {
                                var line = new string(buffer, i + 1, bytesRead - i - 1).TrimEnd();
                                if (!string.IsNullOrEmpty(line))
                                {
                                    lines.Insert(0, line);
                                    if (lines.Count >= NUMER_OF_LINES)
                                    {
                                        break;
                                    }
                                }
                                bytesRead = i;
                            }
                        }

                        position -= readLength;
                    }

                    if (position == 0 && lines.Count < NUMER_OF_LINES)
                    {
                        fileStream.Seek(0, SeekOrigin.Begin);
                        bytesRead = reader.ReadBlock(buffer, 0, (int)fileStream.Length);
                        var content = new string(buffer, 0, bytesRead).TrimEnd();
                        var allLines = content.Split(new[] { '\r', '\n' }, StringSplitOptions.RemoveEmptyEntries);
                        lines.InsertRange(0, allLines.Take(NUMER_OF_LINES - lines.Count));
                    }

                    return NormalizeLineEndings(string.Join(Environment.NewLine, lines));
                }
            }
            catch (IOException ex)
            {
                MessageBox.Show($"Error reading log file: {ex.Message}", "Error", MessageBoxButtons.OK, MessageBoxIcon.Error);
                return string.Empty;
            }
        }



        private string NormalizeLineEndings(string text)
        {
            // Normalize all line endings to the platform-specific newline sequence (CRLF on Windows)
            return text.Replace("\r\n", "\n").Replace("\n", Environment.NewLine);
        }

        private void DisplayFormattedLogContent(string content)
        {
            txtLogViewer.Clear();
            string[] lines = content.Split(new[] { Environment.NewLine }, StringSplitOptions.None);
            foreach (var line in lines)
            {
                if (string.IsNullOrWhiteSpace(line)) continue;

                int timestampLength = "yyyy/MM/dd HH:mm:ss".Length;
                if (line.Length >= timestampLength + 4 && DateTime.TryParse(line.Substring(0, timestampLength), out _))
                {
                    // Timestamp
                    txtLogViewer.SelectionColor = System.Drawing.Color.Gray;
                    txtLogViewer.AppendText(line.Substring(0, timestampLength + 1));

                    // Log level
                    string logLevel = line.Substring(timestampLength + 1, 3);
                    System.Drawing.Color logColor = System.Drawing.Color.White;

                    switch (logLevel)
                    {
                        case "INF":
                            logColor = System.Drawing.Color.ForestGreen;
                            break;
                        case "DEB":
                            logColor = System.Drawing.Color.LightBlue;
                            break;
                        case "DBG":
                            logColor = System.Drawing.Color.Beige;
                            break;
                        case "WAR":
                            logColor = System.Drawing.Color.Yellow;
                            break;
                        case "ERR":
                            logColor = System.Drawing.Color.Red;
                            break;
                    }

                    txtLogViewer.SelectionColor = logColor;
                    txtLogViewer.AppendText(logLevel);

                    // Rest of the message
                    string message = line.Substring(timestampLength + 4);
                    txtLogViewer.SelectionColor = System.Drawing.Color.White;
                    txtLogViewer.AppendText(" " + message + Environment.NewLine);
                }
                else
                {
                    txtLogViewer.SelectionColor = System.Drawing.Color.White;
                    txtLogViewer.AppendText(line + Environment.NewLine);
                }
            }

            txtLogViewer.SelectionStart = txtLogViewer.Text.Length;
            txtLogViewer.ScrollToCaret();
        }


        private void btnStart_Click(object sender, EventArgs e)
        {
            ExecuteWinSWCommand("start");
            UpdateServiceStatus();
            RefreshLog();

        }

        private void btnStop_Click(object sender, EventArgs e)
        {
            ExecuteWinSWCommand("stop");
            UpdateServiceStatus();
            RefreshLog();

        }

        private void btnRestart_Click(object sender, EventArgs e)
        {
            ExecuteWinSWCommand("restart");
            UpdateServiceStatus();
            RefreshLog();

        }

        private void btnInstall_Click(object sender, EventArgs e)
        {
            ExecuteWinSWCommand("install");
            UpdateServiceStatus();

        }

        private void btnUninstall_Click(object sender, EventArgs e)
        {
            ExecuteWinSWCommand("uninstall");
            UpdateServiceStatus();
        }

        private void btnOpenLogs_Click(object sender, EventArgs e)
        {
            Process.Start("explorer.exe", LOG_FOLDER_PATH);
        }

        private void btnRefreshLog_Click(object sender, EventArgs e)
        {
            RefreshLog();
        }


        private void btnEditMtxConfig_Click(object sender, EventArgs e)
        {
            string configPath = Path.Combine(AppDomain.CurrentDomain.BaseDirectory, MEDIAMTX_CONFIG_FILE);
            Process.Start("notepad.exe", configPath);
        }

        private void ExecuteWinSWCommand(string command)
        {
            string winSWPath = Path.Combine(AppDomain.CurrentDomain.BaseDirectory, SERVICE_EXE_FILE);
            ProcessStartInfo psi = new ProcessStartInfo(winSWPath, command)
            {
                UseShellExecute = false,
                RedirectStandardOutput = true,
                RedirectStandardError = true,
                CreateNoWindow = true
            };

            Process process = Process.Start(psi);
            process.WaitForExit();

            string output = process.StandardOutput.ReadToEnd();
            string error = process.StandardError.ReadToEnd();

            if (!string.IsNullOrEmpty(output))
                MessageBox.Show(output, "Output", MessageBoxButtons.OK, MessageBoxIcon.Information);

            if (!string.IsNullOrEmpty(error))
                MessageBox.Show(error, "Error", MessageBoxButtons.OK, MessageBoxIcon.Error);
        }

        private void UpdateServiceStatus()
        {
            try
            {
                ServiceController sc = new ServiceController(serviceName);
                lblStatus.Text = $"Status: {sc.Status}";
            }
            catch (Exception)
            {
                lblStatus.Text = "Service Status: Not Installed";
            }
        }
    }
}
