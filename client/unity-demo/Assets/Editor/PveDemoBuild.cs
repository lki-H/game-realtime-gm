using System;
using System.IO;
using UnityEditor;
using UnityEditor.SceneManagement;
using UnityEngine;

public static class PveDemoBuild
{
    public static void Windows()
    {
        var args = Environment.GetCommandLineArgs();
        string output = null;
        for (var index = 0; index < args.Length - 1; index++)
            if (args[index] == "-pveBuildOutput") output = args[index + 1];
        if (string.IsNullOrWhiteSpace(output)) throw new ArgumentException("-pveBuildOutput is required");
        output = Path.GetFullPath(output);
        Directory.CreateDirectory(Path.GetDirectoryName(output));
        Directory.CreateDirectory("Assets/Scenes");
        if (!File.Exists("Assets/Scenes/PveControl.unity"))
        {
            var scene = EditorSceneManager.NewScene(NewSceneSetup.DefaultGameObjects, NewSceneMode.Single);
            if (!EditorSceneManager.SaveScene(scene, "Assets/Scenes/PveControl.unity")) throw new InvalidOperationException("Scene save failed");
        }
        var report = BuildPipeline.BuildPlayer(new BuildPlayerOptions
        {
            scenes = new[] { "Assets/Scenes/PveControl.unity" },
            locationPathName = output,
            target = BuildTarget.StandaloneWindows64,
            options = BuildOptions.None
        });
        if (report.summary.result != UnityEditor.Build.Reporting.BuildResult.Succeeded) throw new InvalidOperationException("Windows build failed: " + report.summary.result);
        Debug.Log("PVE Windows build succeeded; bytes=" + report.summary.totalSize);
    }
}
