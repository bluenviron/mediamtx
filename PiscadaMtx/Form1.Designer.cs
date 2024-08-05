namespace PiscadaMtx
{
    partial class Form1
    {
        private System.ComponentModel.IContainer components = null;
        private System.Windows.Forms.Button btnStart;
        private System.Windows.Forms.Button btnStop;
        private System.Windows.Forms.Button btnRestart;
        private System.Windows.Forms.Button btnInstall;
        private System.Windows.Forms.Button btnUninstall;
        private System.Windows.Forms.Button btnOpenLogs;
        private System.Windows.Forms.Button btnEditConfig;
        private System.Windows.Forms.Button btnRefreshLog;
        private System.Windows.Forms.RichTextBox txtLogViewer;
        private System.Windows.Forms.Label lblStatus;

        protected override void Dispose(bool disposing)
        {
            if (disposing && (components != null))
            {
                components.Dispose();
            }
            base.Dispose(disposing);
        }

        private void InitializeComponent()
        {
            this.btnStart = new System.Windows.Forms.Button();
            this.btnStop = new System.Windows.Forms.Button();
            this.btnRestart = new System.Windows.Forms.Button();
            this.btnInstall = new System.Windows.Forms.Button();
            this.btnUninstall = new System.Windows.Forms.Button();
            this.btnOpenLogs = new System.Windows.Forms.Button();
            this.btnEditConfig = new System.Windows.Forms.Button();
            this.btnRefreshLog = new System.Windows.Forms.Button();
            this.txtLogViewer = new System.Windows.Forms.RichTextBox();
            this.lblStatus = new System.Windows.Forms.Label();
            this.SuspendLayout();
            // 
            // btnStart
            // 
            this.btnStart.Anchor = ((System.Windows.Forms.AnchorStyles)((System.Windows.Forms.AnchorStyles.Top | System.Windows.Forms.AnchorStyles.Left)));
            this.btnStart.Location = new System.Drawing.Point(12, 12);
            this.btnStart.Name = "btnStart";
            this.btnStart.Size = new System.Drawing.Size(75, 23);
            this.btnStart.TabIndex = 0;
            this.btnStart.Text = "Start";
            this.btnStart.UseVisualStyleBackColor = true;
            this.btnStart.Click += new System.EventHandler(this.btnStart_Click);
            // 
            // btnStop
            // 
            this.btnStop.Anchor = ((System.Windows.Forms.AnchorStyles)((System.Windows.Forms.AnchorStyles.Top | System.Windows.Forms.AnchorStyles.Left)));
            this.btnStop.Location = new System.Drawing.Point(93, 12);
            this.btnStop.Name = "btnStop";
            this.btnStop.Size = new System.Drawing.Size(75, 23);
            this.btnStop.TabIndex = 1;
            this.btnStop.Text = "Stop";
            this.btnStop.UseVisualStyleBackColor = true;
            this.btnStop.Click += new System.EventHandler(this.btnStop_Click);
            // 
            // btnRestart
            // 
            this.btnRestart.Anchor = ((System.Windows.Forms.AnchorStyles)((System.Windows.Forms.AnchorStyles.Top | System.Windows.Forms.AnchorStyles.Left)));
            this.btnRestart.Location = new System.Drawing.Point(174, 12);
            this.btnRestart.Name = "btnRestart";
            this.btnRestart.Size = new System.Drawing.Size(75, 23);
            this.btnRestart.TabIndex = 2;
            this.btnRestart.Text = "Restart";
            this.btnRestart.UseVisualStyleBackColor = true;
            this.btnRestart.Click += new System.EventHandler(this.btnRestart_Click);
            // 
            // lblStatus
            // 
            this.lblStatus.Anchor = ((System.Windows.Forms.AnchorStyles)((System.Windows.Forms.AnchorStyles.Top | System.Windows.Forms.AnchorStyles.Left)));
            this.lblStatus.AutoSize = true;
            this.lblStatus.Location = new System.Drawing.Point(12, 48);
            this.lblStatus.Name = "lblStatus";
            this.lblStatus.Size = new System.Drawing.Size(40, 13);
            this.lblStatus.TabIndex = 3;
            this.lblStatus.Text = "Status: ";
            // 
            // btnInstall
            // 
            this.btnInstall.Anchor = ((System.Windows.Forms.AnchorStyles)((System.Windows.Forms.AnchorStyles.Bottom | System.Windows.Forms.AnchorStyles.Left)));
            this.btnInstall.Location = new System.Drawing.Point(12, 437);
            this.btnInstall.Name = "btnInstall";
            this.btnInstall.Size = new System.Drawing.Size(120, 23);
            this.btnInstall.TabIndex = 6;
            this.btnInstall.Text = "Install MTX Service";
            this.btnInstall.UseVisualStyleBackColor = true;
            this.btnInstall.Click += new System.EventHandler(this.btnInstall_Click);
            // 
            // btnUninstall
            // 
            this.btnUninstall.Anchor = ((System.Windows.Forms.AnchorStyles)((System.Windows.Forms.AnchorStyles.Bottom | System.Windows.Forms.AnchorStyles.Left)));
            this.btnUninstall.Location = new System.Drawing.Point(138, 437);
            this.btnUninstall.Name = "btnUninstall";
            this.btnUninstall.Size = new System.Drawing.Size(120, 23);
            this.btnUninstall.TabIndex = 7;
            this.btnUninstall.Text = "Uninstall MTX Service";
            this.btnUninstall.UseVisualStyleBackColor = true;
            this.btnUninstall.Click += new System.EventHandler(this.btnUninstall_Click);
            // 
            // btnOpenLogs
            // 
            this.btnOpenLogs.Anchor = ((System.Windows.Forms.AnchorStyles)((System.Windows.Forms.AnchorStyles.Top | System.Windows.Forms.AnchorStyles.Right)));
            this.btnOpenLogs.Location = new System.Drawing.Point(663, 12);
            this.btnOpenLogs.Name = "btnOpenLogs";
            this.btnOpenLogs.Size = new System.Drawing.Size(125, 23);
            this.btnOpenLogs.TabIndex = 4;
            this.btnOpenLogs.Text = "Open Logs Folder";
            this.btnOpenLogs.UseVisualStyleBackColor = true;
            this.btnOpenLogs.Click += new System.EventHandler(this.btnOpenLogs_Click);

            //
            // btnEditConfig
            //
            this.btnEditConfig.Anchor = ((System.Windows.Forms.AnchorStyles)((System.Windows.Forms.AnchorStyles.Top | System.Windows.Forms.AnchorStyles.Right)));
            this.btnEditConfig.Location = new System.Drawing.Point(530, 12);
            this.btnEditConfig.Name = "btnEditConfig";
            this.btnEditConfig.Size = new System.Drawing.Size(125, 23);
            this.btnEditConfig.TabIndex = 5;
            this.btnEditConfig.Text = "Edit MTX config";
            this.btnEditConfig.UseVisualStyleBackColor = true;
            this.btnEditConfig.Click += new System.EventHandler(this.btnEditMtxConfig_Click);

            // 
            // btnRefreshLog
            // 
            this.btnRefreshLog.Anchor = ((System.Windows.Forms.AnchorStyles)((System.Windows.Forms.AnchorStyles.Bottom | System.Windows.Forms.AnchorStyles.Right)));
            this.btnRefreshLog.Location = new System.Drawing.Point(663, 437);
            this.btnRefreshLog.Name = "btnRefreshLog";
            this.btnRefreshLog.Size = new System.Drawing.Size(125, 23);
            this.btnRefreshLog.TabIndex = 9;
            this.btnRefreshLog.Text = "Refresh Log";
            this.btnRefreshLog.UseVisualStyleBackColor = true;
            this.btnRefreshLog.Click += new System.EventHandler(this.btnRefreshLog_Click);
            // 
            // txtLogViewer
            // 
            this.txtLogViewer = new System.Windows.Forms.RichTextBox();
            this.txtLogViewer.Anchor = ((System.Windows.Forms.AnchorStyles)((((System.Windows.Forms.AnchorStyles.Top | System.Windows.Forms.AnchorStyles.Bottom)
            | System.Windows.Forms.AnchorStyles.Left)
            | System.Windows.Forms.AnchorStyles.Right)));
            this.txtLogViewer.Location = new System.Drawing.Point(12, 70);
            this.txtLogViewer.Multiline = true;
            this.txtLogViewer.Name = "txtLogViewer";
            this.txtLogViewer.ScrollBars = System.Windows.Forms.RichTextBoxScrollBars.Vertical;
            this.txtLogViewer.Size = new System.Drawing.Size(776, 361);
            this.txtLogViewer.TabIndex = 8;
            this.txtLogViewer.BackColor = System.Drawing.Color.Black;
            this.txtLogViewer.ForeColor = System.Drawing.Color.White;
            this.txtLogViewer.ReadOnly = true;
            this.txtLogViewer.Font = new System.Drawing.Font("Consolas", 10); // Set font to Consolas 16px

            // 
            // Form1
            // 
            this.AutoScaleDimensions = new System.Drawing.SizeF(6F, 13F);
            this.AutoScaleMode = System.Windows.Forms.AutoScaleMode.Font;
            this.ClientSize = new System.Drawing.Size(800, 472);
            this.MinimumSize = new System.Drawing.Size(800, 472); // Set minimum size here
            this.Controls.Add(this.lblStatus);
            this.Controls.Add(this.btnInstall);
            this.Controls.Add(this.btnUninstall);
            this.Controls.Add(this.txtLogViewer);
            this.Controls.Add(this.btnRefreshLog);
            this.Controls.Add(this.btnOpenLogs);
            this.Controls.Add(this.btnEditConfig);
            this.Controls.Add(this.btnRestart);
            this.Controls.Add(this.btnStop);
            this.Controls.Add(this.btnStart);
            this.Name = "Form1";
            this.Text = "Piscada MTX BufferBoss";
            this.Icon = this.Icon = new Icon(Path.Combine(Application.StartupPath, "buffer_boss_icon.ico"));
            this.ResumeLayout(false);
            this.PerformLayout();
            this.RefreshLog();
        }
    }
}
