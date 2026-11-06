using System.Runtime.CompilerServices;
using System.Runtime.Loader;
using System.Text;
using WeChatOcr;

if (args.Length != 2)
{
    Console.Error.WriteLine("Expected an image path and a WeChat OCR component directory.");
    return 2;
}

var componentDirectory = Path.GetFullPath(args[1]);
AssemblyLoadContext.Default.Resolving += (context, name) =>
{
    var path = Path.Combine(componentDirectory, name.Name + ".dll");
    return File.Exists(path) ? context.LoadFromAssemblyPath(path) : null;
};

try
{
    Console.OutputEncoding = new UTF8Encoding(false);
    Console.Write(await RecognizeAsync(Path.GetFullPath(args[0]), componentDirectory));
    return 0;
}
catch (Exception error)
{
    Console.Error.WriteLine(error.GetBaseException().Message);
    return 1;
}

[MethodImpl(MethodImplOptions.NoInlining)]
static async Task<string> RecognizeAsync(string imagePath, string componentDirectory)
{
    DataLocation.SetBaseDirectory(componentDirectory);
    using var engine = new ImageOcr();
    var completion = new TaskCompletionSource<string>(TaskCreationOptions.RunContinuationsAsynchronously);
    engine.Run(imagePath, (_, result) =>
    {
        var lines = result?.OcrResult?.SingleResult;
        if (lines is null)
        {
            completion.TrySetException(new InvalidOperationException("The OCR engine returned no result."));
            return;
        }

        completion.TrySetResult(string.Join(Environment.NewLine,
            lines.Select(line => line?.SingleStrUtf8).Where(text => !string.IsNullOrWhiteSpace(text))));
    });
    return await completion.Task.WaitAsync(TimeSpan.FromSeconds(12));
}
